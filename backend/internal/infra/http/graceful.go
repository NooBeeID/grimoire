package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"grimoire/internal/infra/config"
)

// Server membungkus http.Server.
type Server struct{ *http.Server }

// NewServer membuat server tanpa WriteTimeout (SSE/streaming MCP).
func NewServer(cfg config.Config, log *slog.Logger, h http.Handler) *Server {
	return &Server{&http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           h,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}}
}

// Serve menjalankan server sampai ctx selesai, lalu Shutdown dengan batas
// shutdownTimeout; bila timeout, koneksi tersisa diputus lewat Close.
func Serve(ctx context.Context, s *Server, ln net.Listener, shutdownTimeout time.Duration, log *slog.Logger) error {
	if ln == nil {
		var err error
		if ln, err = net.Listen("tcp", s.Addr); err != nil {
			return fmt.Errorf("listen %s: %w", s.Addr, err)
		}
	}
	errc := make(chan error, 1)
	go func() { errc <- s.Serve(ln) }()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	log.Info("http server shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutErr := s.Shutdown(sctx)
	if shutErr != nil {
		_ = s.Close()
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if shutErr != nil {
		return fmt.Errorf("http shutdown: %w", shutErr)
	}
	return nil
}

// ShutdownStep adalah satu langkah shutdown berurutan.
type ShutdownStep struct {
	Name    string
	Timeout time.Duration
	Fn      func(ctx context.Context) error
}

// RunShutdown menjalankan langkah berurutan; error satu langkah tidak menghentikan
// langkah berikutnya. Mengembalikan errors.Join semua error.
func RunShutdown(log *slog.Logger, steps ...ShutdownStep) error {
	var errsAll []error
	for _, st := range steps {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), st.Timeout)
		err := st.Fn(ctx)
		cancel()
		if err != nil {
			log.Error("shutdown step failed", "step", st.Name, "error", err)
			errsAll = append(errsAll, fmt.Errorf("%s: %w", st.Name, err))
			continue
		}
		log.Info("shutdown step done", "step", st.Name, "duration_ms", time.Since(start).Milliseconds())
	}
	return errors.Join(errsAll...)
}
