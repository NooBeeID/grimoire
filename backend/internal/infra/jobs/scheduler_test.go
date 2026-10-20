package jobs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grimoire/lib/common/reqctx"
)

type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if v := reqctx.TraceID(ctx); v != "" {
		r.AddAttrs(slog.String("trace_id", v))
	}
	return h.Handler.Handle(ctx, r)
}
func (h traceHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(a)}
}
func (h traceHandler) WithGroup(n string) slog.Handler { return traceHandler{h.Handler.WithGroup(n)} }

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func newLogger() (*slog.Logger, *syncBuf) {
	buf := &syncBuf{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(traceHandler{h}), buf
}

type fakeLocker struct {
	ok        bool
	lockErr   error
	unlockErr error

	mu     sync.Mutex
	events []string
	// unlockCtxErr merekam ctx.Err() saat unlock dipanggil.
	unlockCtxErr []error
}

func (f *fakeLocker) rec(e string) { f.mu.Lock(); f.events = append(f.events, e); f.mu.Unlock() }
func (f *fakeLocker) Events() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func (f *fakeLocker) TryLock(ctx context.Context, name string) (func(context.Context) error, bool, error) {
	if f.lockErr != nil {
		return nil, false, f.lockErr
	}
	if !f.ok {
		f.rec("skip")
		return nil, false, nil
	}
	f.rec("lock")
	return func(uctx context.Context) error {
		f.mu.Lock()
		f.unlockCtxErr = append(f.unlockCtxErr, uctx.Err())
		f.mu.Unlock()
		f.rec("unlock")
		return f.unlockErr
	}, true, nil
}

func newTest(l locker) (*Scheduler, *syncBuf) {
	log, buf := newLogger()
	return newWithLocker(l, log), buf
}

func stopOK(t *testing.T, s *Scheduler) {
	t.Helper()
	ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
	defer c()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		p := recover()
		if p == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if !strings.Contains(p.(string), want) {
			t.Fatalf("panic %q does not contain %q", p, want)
		}
	}()
	f()
}

func nop(context.Context) error { return nil }

func TestRegister_DuplicatePanics(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	s.Register(Job{Name: "a", Every: time.Hour, Run: nop})
	mustPanic(t, `jobs: job "a" registered twice`, func() { s.Register(Job{Name: "a", Every: time.Hour, Run: nop}) })
}

func TestRegister_InvalidPanics(t *testing.T) {
	cases := map[string]Job{
		"empty name":   {Every: time.Hour, Run: nop},
		"uppercase":    {Name: "Abc", Every: time.Hour, Run: nop},
		"space":        {Name: "a b", Every: time.Hour, Run: nop},
		"nil run":      {Name: "a", Every: time.Hour},
		"both":         {Name: "a", Every: time.Hour, DailyAt: "03:00", Run: nop},
		"neither":      {Name: "a", Run: nop},
		"negative":     {Name: "a", Every: -time.Second, Run: nop},
		"bad daily at": {Name: "a", DailyAt: "25:00", Run: nop},
	}
	for name, j := range cases {
		t.Run(name, func(t *testing.T) {
			s, _ := newTest(&fakeLocker{ok: true})
			mustPanic(t, "jobs:", func() { s.Register(j) })
		})
	}
}

func TestRegister_AfterStartPanics(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	s.Start(context.Background())
	defer stopOK(t, s)
	mustPanic(t, "Register after Start", func() { s.Register(Job{Name: "a", Every: time.Hour, Run: nop}) })
}

func TestStart_Twice_Panics(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	s.Start(context.Background())
	defer stopOK(t, s)
	mustPanic(t, "Start called twice", func() { s.Start(context.Background()) })
}

func TestStart_NoJobs_StopOK(t *testing.T) {
	s, buf := newTest(&fakeLocker{ok: true})
	s.Start(context.Background())
	stopOK(t, s)
	if !strings.Contains(buf.String(), "jobs started") || !strings.Contains(buf.String(), "count=0") {
		t.Fatalf("log: %s", buf)
	}
}

func TestJobRuns_OnEverySchedule(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	var n atomic.Int32
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(context.Context) error { n.Add(1); return nil }})
	s.Start(context.Background())
	time.Sleep(250 * time.Millisecond)
	stopOK(t, s)
	if n.Load() < 3 {
		t.Fatalf("runs=%d", n.Load())
	}
}

func TestJobPanicDoesNotStopScheduler(t *testing.T) {
	s, buf := newTest(&fakeLocker{ok: true})
	var a, b atomic.Int32
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(context.Context) error { a.Add(1); panic("boom") }})
	s.Register(Job{Name: "b", Every: 20 * time.Millisecond, Run: func(context.Context) error { b.Add(1); return nil }})
	s.Start(context.Background())
	time.Sleep(250 * time.Millisecond)
	stopOK(t, s)
	if a.Load() < 2 || b.Load() < 3 {
		t.Fatalf("a=%d b=%d", a.Load(), b.Load())
	}
	out := buf.String()
	for _, w := range []string{"status=panic", "job=a", "stack=", "job panicked"} {
		if !strings.Contains(out, w) {
			t.Fatalf("log missing %q:\n%s", w, out)
		}
	}
}

func TestJobError_LoggedAndContinues(t *testing.T) {
	s, buf := newTest(&fakeLocker{ok: true})
	var n atomic.Int32
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(context.Context) error { n.Add(1); return errors.New("kaboom") }})
	s.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	stopOK(t, s)
	if n.Load() < 2 {
		t.Fatalf("runs=%d", n.Load())
	}
	if !strings.Contains(buf.String(), "status=error") || !strings.Contains(buf.String(), "kaboom") {
		t.Fatalf("log: %s", buf)
	}
}

func TestExecute_NewTraceIDPerRun(t *testing.T) {
	s, buf := newTest(&fakeLocker{ok: true})
	var mu sync.Mutex
	var ids []string
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(ctx context.Context) error {
		mu.Lock()
		ids = append(ids, reqctx.TraceID(ctx))
		mu.Unlock()
		return nil
	}})
	s.Start(context.Background())
	time.Sleep(150 * time.Millisecond)
	stopOK(t, s)
	mu.Lock()
	defer mu.Unlock()
	if len(ids) < 2 {
		t.Fatalf("runs=%d", len(ids))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !strings.HasPrefix(id, "job-") || seen[id] {
			t.Fatalf("bad/duplicate trace id %q", id)
		}
		seen[id] = true
		if !strings.Contains(buf.String(), "trace_id="+id) {
			t.Fatalf("log lacks trace_id=%s", id)
		}
	}
}

func TestExecute_LockedIsSkipped(t *testing.T) {
	f := &fakeLocker{ok: false}
	s, buf := newTest(f)
	var n atomic.Int32
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(context.Context) error { n.Add(1); return nil }})
	s.Start(context.Background())
	time.Sleep(150 * time.Millisecond)
	stopOK(t, s)
	if n.Load() != 0 {
		t.Fatal("Run must not be called")
	}
	if len(f.Events()) < 2 {
		t.Fatalf("loop should keep ticking: %v", f.Events())
	}
	if !strings.Contains(buf.String(), "level=DEBUG msg=\"job finished\"") || !strings.Contains(buf.String(), "status=skipped") {
		t.Fatalf("log: %s", buf)
	}
}

func TestExecute_LockError_NotRun(t *testing.T) {
	s, buf := newTest(&fakeLocker{lockErr: errors.New("db down")})
	var n atomic.Int32
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(context.Context) error { n.Add(1); return nil }})
	s.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	stopOK(t, s)
	if n.Load() != 0 {
		t.Fatal("Run must not be called")
	}
	out := buf.String()
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "status=error") || !strings.Contains(out, "jobs: acquire lock") {
		t.Fatalf("log: %s", out)
	}
}

func TestExecute_UnlockCalledAfterRun(t *testing.T) {
	for name, run := range map[string]func(context.Context) error{
		"ok":    func(context.Context) error { return nil },
		"error": func(context.Context) error { return errors.New("x") },
		"panic": func(context.Context) error { panic("x") },
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeLocker{ok: true}
			s, _ := newTest(f)
			s.runCtx, s.cancel = context.WithCancel(context.Background())
			s.cancel() // runCtx sudah batal: unlock tetap harus memakai ctx hidup
			j := Job{Name: "a", Every: time.Hour, Run: func(ctx context.Context) error { f.rec("run"); return run(ctx) }}
			s.execute(j)
			ev := f.Events()
			if len(ev) != 3 || ev[0] != "lock" || ev[1] != "run" || ev[2] != "unlock" {
				t.Fatalf("events=%v", ev)
			}
			if f.unlockCtxErr[0] != nil {
				t.Fatalf("unlock ctx canceled: %v", f.unlockCtxErr[0])
			}
		})
	}
}

func TestExecute_UnlockErrorLogged(t *testing.T) {
	f := &fakeLocker{ok: true, unlockErr: errors.New("nope")}
	s, buf := newTest(f)
	s.runCtx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()
	s.execute(Job{Name: "a", Every: time.Hour, Run: nop})
	out := buf.String()
	if !strings.Contains(out, "job unlock failed") || !strings.Contains(out, "status=ok") {
		t.Fatalf("log: %s", out)
	}
}

func TestStop_WaitsRunningJob(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	started := make(chan struct{})
	release := make(chan struct{})
	var n atomic.Int32
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(context.Context) error {
		if n.Add(1) == 1 {
			close(started)
			<-release
		}
		return nil
	}})
	s.Start(context.Background())
	<-started
	res := make(chan error, 1)
	go func() {
		ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		res <- s.Stop(ctx)
	}()
	select {
	case err := <-res:
		t.Fatalf("Stop returned early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-res; err != nil {
		t.Fatal(err)
	}
	after := n.Load()
	time.Sleep(100 * time.Millisecond)
	if n.Load() != after {
		t.Fatal("job ran after Stop")
	}
}

func TestStop_TimeoutCancelsRunCtx(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	started := make(chan struct{})
	canceled := make(chan struct{})
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	}})
	s.Start(context.Background())
	<-started
	ctx, c := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer c()
	err := s.Stop(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("Run did not see ctx cancel")
	}
}

func TestStop_Idempotent_AndBeforeStart(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	s2, _ := newTest(&fakeLocker{ok: true})
	s2.Register(Job{Name: "a", Every: time.Hour, Run: nop})
	s2.Start(context.Background())
	stopOK(t, s2)
	stopOK(t, s2)
}

func TestLoop_MissedTicksNotQueued(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	var n atomic.Int32
	s.Register(Job{Name: "a", Every: 20 * time.Millisecond, Run: func(context.Context) error {
		n.Add(1)
		time.Sleep(100 * time.Millisecond)
		return nil
	}})
	s.Start(context.Background())
	time.Sleep(300 * time.Millisecond)
	stopOK(t, s)
	if n.Load() > 3 {
		t.Fatalf("runs=%d, ticks piled up", n.Load())
	}
}

func TestStopInterruptsWaiting(t *testing.T) {
	s, _ := newTest(&fakeLocker{ok: true})
	s.Register(Job{Name: "a", DailyAt: "03:00", Run: nop})
	s.Start(context.Background())
	begin := time.Now()
	stopOK(t, s)
	if d := time.Since(begin); d > 100*time.Millisecond {
		t.Fatalf("Stop took %v", d)
	}
}
