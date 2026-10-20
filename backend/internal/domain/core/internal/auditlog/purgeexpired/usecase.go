package purgeexpired

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

type repository interface {
	DeleteBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error)
}

type Usecase struct {
	repo repository
	opt  Options
	now  func() time.Time
	log  *slog.Logger
}

// New merakit usecase. Retention <= 0 adalah kesalahan wiring (panic) agar tidak menghapus semuanya.
func New(repo repository, opt Options, now func() time.Time, log *slog.Logger) *Usecase {
	if opt.Retention <= 0 {
		panic("purgeexpired: Retention must be > 0")
	}
	if opt.BatchSize <= 0 {
		opt.BatchSize = BatchSize
	}
	if opt.Pause < 0 {
		opt.Pause = 0
	}
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{repo: repo, opt: opt, now: now, log: log}
}

// Run adalah isi job. Cutoff dihitung sekali; loop sampai batch parsial atau ctx selesai.
func (u *Usecase) Run(ctx context.Context) error {
	wall := time.Now()
	cutoff := base.PurgeCutoff(u.now(), u.opt.Retention)
	var total int64
	var batches int

	attrs := func(err error) []any {
		a := []any{
			slog.Int64("total_deleted", total),
			slog.Int("batches", batches),
			slog.Float64("duration_ms", float64(time.Since(wall))/float64(time.Millisecond)),
		}
		if err != nil {
			a = append(a, slog.String("err", err.Error()))
		}
		return a
	}
	interrupted := func(err error) error {
		u.log.Info("audit purge interrupted", attrs(err)...)
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			return interrupted(err)
		}
		n, err := u.repo.DeleteBatch(ctx, cutoff, u.opt.BatchSize)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil && errors.Is(err, cerr) {
				return interrupted(err)
			}
			u.log.Error("audit purge failed", attrs(err)...)
			return err
		}
		total += n
		batches++
		if n < int64(u.opt.BatchSize) {
			break
		}
		if u.opt.Pause > 0 {
			t := time.NewTimer(u.opt.Pause)
			select {
			case <-ctx.Done():
				t.Stop()
				return interrupted(ctx.Err())
			case <-t.C:
			}
		}
	}
	u.log.Info("audit purge done", attrs(nil)...)
	return nil
}
