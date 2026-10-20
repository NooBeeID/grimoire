// Package jobs adalah scheduler in-process yang aman multi-instance.
//
// Jadwal Every disejajarkan ke jam dinding (kelipatan Every sejak Unix epoch),
// DailyAt "HH:MM" dalam UTC. Job Every tidak jalan saat start. Setiap job punya satu
// goroutine yang mengeksekusi sinkron, sehingga tick yang terlewat dilewati (tidak
// menumpuk). Tiap eksekusi mengambil pg_try_advisory_lock pada koneksi khusus (satu
// koneksi pool selama job berjalan); lock hanya mencegah tumpang tindih, jadi Run
// WAJIB idempoten. Panic di Run di-recover. Stop menunggu job yang berjalan sampai
// deadline ctx, lalu membatalkan ctx job.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"grimoire/lib/common/reqctx"
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Job adalah satu pekerjaan terjadwal.
type Job struct {
	Name    string        // unik, ^[a-z0-9][a-z0-9._-]*$
	Every   time.Duration // > 0, epoch-aligned. Tepat salah satu dari Every / DailyAt
	DailyAt string        // "HH:MM" UTC
	// Run WAJIB idempoten (lock hanya mencegah tumpang tindih, bukan eksekusi ganda
	// antar instance) dan menghormati ctx. Error = status "error" (di-log).
	Run func(ctx context.Context) error
}

type Scheduler struct {
	mu      sync.Mutex
	jobs    map[string]Job
	order   []string
	started bool
	stopped bool

	locker locker
	log    *slog.Logger
	now    func() time.Time

	stopCh  chan struct{}
	runCtx  context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running atomic.Int32
}

// New membuat Scheduler dengan lock advisory Postgres. d biasanya *db.DB.
func New(d connProvider, log *slog.Logger) *Scheduler {
	return newWithLocker(pgLocker{db: d}, log)
}

func newWithLocker(l locker, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		jobs:   map[string]Job{},
		locker: l,
		log:    log,
		now:    time.Now,
		stopCh: make(chan struct{}),
	}
}

// Register mendaftarkan job sebelum Start. Panic bila job invalid, duplikat, atau setelah Start.
func (s *Scheduler) Register(j Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		panic(fmt.Sprintf("jobs: Register after Start (job %q)", j.Name))
	}
	if !nameRe.MatchString(j.Name) {
		panic(fmt.Sprintf("jobs: invalid job name %q", j.Name))
	}
	if j.Run == nil {
		panic(fmt.Sprintf("jobs: job %q: Run is required", j.Name))
	}
	if j.Every < 0 {
		panic(fmt.Sprintf("jobs: job %q: Every must be positive", j.Name))
	}
	if (j.Every > 0) == (j.DailyAt != "") {
		panic(fmt.Sprintf("jobs: job %q: exactly one of Every or DailyAt is required", j.Name))
	}
	if j.DailyAt != "" {
		if _, _, err := parseDailyAt(j.DailyAt); err != nil {
			panic(fmt.Sprintf("jobs: job %q: %v", j.Name, err))
		}
	}
	if _, dup := s.jobs[j.Name]; dup {
		panic(fmt.Sprintf("jobs: job %q registered twice", j.Name))
	}
	s.jobs[j.Name] = j
	s.order = append(s.order, j.Name)
}

// Start menjalankan semua job terdaftar. Panic bila dipanggil dua kali.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		panic("jobs: Start called twice")
	}
	s.started = true
	s.runCtx, s.cancel = context.WithCancel(ctx)
	for _, name := range s.order {
		j := s.jobs[name]
		s.wg.Add(1)
		go s.loop(j)
	}
	s.log.Info("jobs started", "count", len(s.order))
}

// Stop berhenti menjadwalkan dan menunggu job yang berjalan sampai deadline ctx.
// Idempoten; sebelum Start mengembalikan nil.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.started || s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	close(s.stopCh)
	s.mu.Unlock()

	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		s.cancel()
		s.log.Info("jobs stopped")
		return nil
	case <-ctx.Done():
		s.cancel()
		return fmt.Errorf("jobs: stop timed out with %d job(s) still running: %w", s.running.Load(), ctx.Err())
	}
}

func (s *Scheduler) loop(j Job) {
	defer s.wg.Done()
	for {
		next := nextRun(s.now(), j)
		s.log.Debug("job scheduled", "job", j.Name, "next_run", next)
		t := time.NewTimer(next.Sub(s.now()))
		select {
		case <-s.stopCh:
			t.Stop()
			return
		case <-s.runCtx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		s.execute(j)
	}
}

func (s *Scheduler) execute(j Job) {
	ctx := reqctx.WithTraceID(s.runCtx, "job-"+uuid.NewString())
	start := s.now()
	status, err := s.runLocked(ctx, j)
	level := slog.LevelInfo
	switch status {
	case "skipped":
		level = slog.LevelDebug
	case "error", "panic":
		level = slog.LevelError
	}
	args := []any{"job", j.Name, "status", status,
		"duration_ms", float64(s.now().Sub(start).Microseconds()) / 1000}
	if err != nil {
		args = append(args, "error", err)
	}
	s.log.Log(ctx, level, "job finished", args...)
}

func (s *Scheduler) runLocked(ctx context.Context, j Job) (string, error) {
	unlock, ok, lerr := s.locker.TryLock(ctx, j.Name)
	if lerr != nil {
		return "error", fmt.Errorf("jobs: acquire lock: %w", lerr)
	}
	if !ok {
		return "skipped", nil
	}
	s.running.Add(1)
	defer s.running.Add(-1)
	defer func() {
		uctx, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		if err := unlock(uctx); err != nil {
			s.log.ErrorContext(ctx, "job unlock failed", "job", j.Name, "error", err)
		}
	}()
	return s.safeRun(ctx, j)
}

func (s *Scheduler) safeRun(ctx context.Context, j Job) (status string, err error) {
	defer func() {
		if p := recover(); p != nil {
			status = "panic"
			err = fmt.Errorf("panic: %v", p)
			s.log.ErrorContext(ctx, "job panicked", "job", j.Name, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		}
	}()
	if rerr := j.Run(ctx); rerr != nil {
		return "error", rerr
	}
	return "ok", nil
}
