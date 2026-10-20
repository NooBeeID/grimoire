package purgesessions

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
)

type repository interface {
	DeleteExpiredSessions(ctx context.Context, now, idleCutoff time.Time) (int64, error)
	DeleteExpiredThrottles(ctx context.Context, now, windowCutoff time.Time) (int64, error)
}

type Usecase struct {
	repo        repository
	clock       base.Clock
	idleTimeout time.Duration
	window      time.Duration
	log         *slog.Logger
}

// New panic bila idle/window tidak positif (kesalahan wiring).
func New(repo repository, clock base.Clock, idle, window time.Duration, log *slog.Logger) *Usecase {
	if idle <= 0 || window <= 0 {
		panic("purgesessions: idle timeout dan window harus > 0")
	}
	return &Usecase{repo: repo, clock: clock, idleTimeout: idle, window: window, log: log}
}

// Run menghapus sesi dan throttle kedaluwarsa. Idempoten.
func (u *Usecase) Run(ctx context.Context) error {
	wall := time.Now()
	start := u.clock()
	idleCutoff := start.Add(-u.idleTimeout)
	windowCutoff := start.Add(-u.window)

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("purgesessions: %w", err)
	}
	ds, err := u.repo.DeleteExpiredSessions(ctx, start, idleCutoff)
	if err != nil {
		return fmt.Errorf("purgesessions: sessions: %w", err)
	}
	dt, err := u.repo.DeleteExpiredThrottles(ctx, start, windowCutoff)
	if err != nil {
		u.log.InfoContext(ctx, "auth purge sessions sebagian", "deleted_sessions", ds)
		return fmt.Errorf("purgesessions: login_throttles: %w", err)
	}
	u.log.InfoContext(ctx, "auth purge sessions selesai",
		"deleted_sessions", ds, "deleted_throttles", dt,
		"duration_ms", time.Since(wall).Milliseconds())
	return nil
}
