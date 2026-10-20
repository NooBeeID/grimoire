package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"grimoire/internal/domain/core"
	"grimoire/internal/domain/discovery"
	"grimoire/internal/domain/project"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/db"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/jobs"
)

// WithDomains merakit db, bus, router dummy, Deps, dan Register semua domain tanpa
// listen HTTP dan tanpa Start scheduler, menjalankan fn, lalu bus.Shutdown + db.Close.
// Tidak menjalankan migrasi. Dipakai CLI. Error fn tetap terbaca lewat errors.As.
func WithDomains(ctx context.Context, cfg config.Config, log *slog.Logger, fn func(app.Deps) error) error {
	d, err := db.Open(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	b := bus.NewRegistry(log)
	sched := jobs.New(d, log)

	srvCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	_, api, mcp := httpx.NewRouter(cfg, log, b, d)
	deps := app.Deps{Ctx: srvCtx, Config: cfg, Log: log, DB: d, Bus: b, Jobs: sched, API: api, MCP: mcp}
	core.Register(deps)
	project.Register(deps)
	discovery.Register(deps)

	ferr := fn(deps)

	cancel()
	var errList []error
	errList = append(errList, ferr)
	bCtx, bcancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.BusShutdownTimeout)
	if err := b.Shutdown(bCtx); err != nil {
		errList = append(errList, fmt.Errorf("shutdown bus: %w", err))
	}
	bcancel()
	if err := d.Close(); err != nil {
		errList = append(errList, fmt.Errorf("shutdown db: %w", err))
	}
	return errors.Join(errList...)
}
