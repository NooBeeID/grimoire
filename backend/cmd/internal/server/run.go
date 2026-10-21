// Package server merakit seluruh infra dan domain menjadi proses yang berjalan.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"grimoire/cmd/internal/buildinfo"
	"grimoire/internal/domain/core"
	"grimoire/internal/domain/discovery"
	"grimoire/internal/domain/project"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/db"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/jobs"
	"grimoire/internal/infra/logger"
	"grimoire/internal/infra/static"
	"grimoire/migrations"
)

// Option mengubah perilaku Run; dipakai test saja.
type Option func(*runOpts)

type runOpts struct {
	listenReady chan<- string
	appHook     func(app.Deps)
}

// WithListenReady melaporkan alamat nyata listener. ch sebaiknya ber-buffer 1;
// pengiriman tidak pernah memblokir.
func WithListenReady(ch chan<- string) Option { return func(o *runOpts) { o.listenReady = ch } }

// WithAppHook dipanggil setelah Register domain dan sebelum static dipasang.
func WithAppHook(fn func(app.Deps)) Option { return func(o *runOpts) { o.appHook = fn } }

// Run merakit dan menjalankan server sampai ctx dibatalkan atau server gagal,
// lalu shutdown berurutan. Mengembalikan nil hanya bila semuanya sukses.
func Run(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...Option) error {
	var o runOpts
	for _, fn := range opts {
		fn(&o)
	}
	log.Info("starting", "version", buildinfo.String(), "config", cfg)

	d, err := db.Open(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	if cfg.RunMigrationsOnStart {
		res, err := db.Migrate(ctx, d, migrations.FS, log)
		if err != nil {
			_ = d.Close()
			return fmt.Errorf("migrate: %w", err)
		}
		log.Info("migrations applied", "count", len(res.Applied))
	}

	b := bus.NewRegistry(log)
	sched := jobs.New(d, log)

	// Ctx server tidak ikut batal saat sinyal; dibatalkan setelah HTTP drain.
	srvCtx, cancelSrv := context.WithCancel(context.WithoutCancel(ctx))
	root, api, mcp := httpx.NewRouter(cfg, log, b, d)
	deps := app.Deps{Ctx: srvCtx, Config: cfg, Log: log, DB: d, Bus: b, Jobs: sched, API: api, MCP: mcp, Version: buildinfo.Version}
	core.Register(deps)
	project.Register(deps)
	discovery.Register(deps)
	if o.appHook != nil {
		o.appHook(deps)
	}

	fail := func(step string, err error) error {
		cancelSrv()
		_ = d.Close()
		return fmt.Errorf("%s: %w", step, err)
	}

	h, err := static.New(cfg, static.Embedded(), log)
	if err != nil {
		return fail("static", err)
	}
	root.NotFound(h.ServeHTTP)

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fail("listen", err)
	}
	sched.Start(srvCtx)

	srv := httpx.NewServer(cfg, log, root)
	srv.BaseContext = func(net.Listener) context.Context { return srvCtx }
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	log.Info("listening", "addr", ln.Addr().String())
	if o.listenReady != nil {
		select {
		case o.listenReady <- ln.Addr().String():
		default:
		}
	}

	var errs []error
	served := false
	select {
	case <-ctx.Done():
	case err := <-errCh:
		served = true
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("serve: %w", err))
		}
	}

	// 1. HTTP drain.
	shCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	if err := srv.Shutdown(shCtx); err != nil {
		log.Error("shutdown: http melewati batas waktu", "error", err)
		errs = append(errs, fmt.Errorf("shutdown http: %w", err))
		_ = srv.Close()
	} else {
		log.Info("shutdown: http selesai")
	}
	cancel()
	if !served {
		<-errCh
	}
	// 2. Ctx server batal.
	cancelSrv()
	// 3. Jobs.
	jCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	if err := sched.Stop(jCtx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown jobs: %w", err))
	} else {
		log.Info("shutdown: jobs berhenti")
	}
	cancel()
	// 4. Bus.
	bCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.BusShutdownTimeout)
	if err := b.Shutdown(bCtx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown bus: %w", err))
	} else {
		log.Info("shutdown: bus selesai")
	}
	cancel()
	// 5. DB.
	if err := d.Close(); err != nil {
		errs = append(errs, fmt.Errorf("shutdown db: %w", err))
	} else {
		log.Info("shutdown: db ditutup")
	}
	return errors.Join(errs...)
}

// MigrateUp membuka DB, menjalankan migrasi ter-embed, dan mencetak hasilnya ke out.
func MigrateUp(ctx context.Context, cfg config.Config, log *slog.Logger, out io.Writer) error {
	d, err := db.Open(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer d.Close()
	res, err := db.Migrate(ctx, d, migrations.FS, log)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if len(res.Applied) == 0 {
		fmt.Fprintln(out, "tidak ada migrasi baru")
		return nil
	}
	for _, n := range res.Applied {
		fmt.Fprintf(out, "diterapkan: %s\n", n)
	}
	return nil
}

// LoadConfig memuat config; bila tidak valid menulis pesan ke stderr (tanpa nilai secret).
func LoadConfig(getenv func(string) string, stderr io.Writer) (config.Config, *slog.Logger, bool) {
	cfg, err := config.Load(getenv)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return config.Config{}, nil, false
	}
	return cfg, logger.New(cfg), true
}

// ServeFromEnv memuat config, menangani sinyal, menjalankan Run, dan mengembalikan exit code.
func ServeFromEnv(ctx context.Context, getenv func(string) string, stderr io.Writer) int {
	cfg, log, ok := LoadConfig(getenv, stderr)
	if !ok {
		return 1
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); stop() }() // sinyal kedua memakai perilaku default
	if err := Run(ctx, cfg, log); err != nil {
		log.Error("server berhenti dengan error", "error", err)
		return 1
	}
	return 0
}

// MigrateFromEnv adalah pembungkus untuk `migrate up`.
func MigrateFromEnv(ctx context.Context, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, log, ok := LoadConfig(getenv, stderr)
	if !ok {
		return 1
	}
	if err := MigrateUp(ctx, cfg, log, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
