// Package bus adalah jalur interaksi in-process antar module/domain.
package bus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
)

// Handler memproses request bus.
type Handler func(ctx context.Context, req any) (any, error)

var (
	ErrNotRegistered   = errors.New("bus: endpoint not registered")
	ErrNilRegistry     = errors.New("bus: nil registry (wiring error)")
	ErrShuttingDown    = errors.New("bus: shutting down")
	ErrBadRequestType  = errors.New("bus: unexpected request type")
	ErrBadResponseType = errors.New("bus: unexpected response type")
)

// Registry memetakan endpoint ke Handler.
type Registry struct {
	log      *slog.Logger
	mu       sync.RWMutex
	handlers map[string]Handler
	life     sync.Mutex
	closed   bool
	inflight int
	wg       sync.WaitGroup
}

// NewRegistry membuat registry; log nil memakai slog.Default().
func NewRegistry(log *slog.Logger) *Registry {
	if log == nil {
		log = slog.Default()
	}
	return &Registry{log: log, handlers: map[string]Handler{}}
}

// Register mendaftarkan handler; panic bila duplikat, handler nil, atau endpoint kosong.
func (r *Registry) Register(endpoint string, h Handler) {
	if endpoint == "" {
		panic("bus: empty endpoint")
	}
	if h == nil {
		panic(fmt.Sprintf("bus: nil handler for endpoint %q", endpoint))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.handlers[endpoint]; ok {
		panic(fmt.Sprintf("bus: endpoint %q registered twice", endpoint))
	}
	r.handlers[endpoint] = h
}

func (r *Registry) lookup(endpoint string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[endpoint]
	return h, ok
}

// Call memanggil handler secara sinkron dengan ctx pemanggil.
func (r *Registry) Call(ctx context.Context, endpoint string, req any) (any, error) {
	if r == nil {
		return nil, ErrNilRegistry
	}
	h, ok := r.lookup(endpoint)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotRegistered, endpoint)
	}
	return h(ctx, req)
}

// Publish menjalankan handler di goroutine (fire-and-forget) tanpa cancel dari parent.
func (r *Registry) Publish(ctx context.Context, endpoint string, req any) error {
	if r == nil {
		return ErrNilRegistry
	}
	r.life.Lock()
	if r.closed {
		r.life.Unlock()
		return ErrShuttingDown
	}
	h, ok := r.lookup(endpoint)
	if !ok {
		r.life.Unlock()
		return fmt.Errorf("%w: %s", ErrNotRegistered, endpoint)
	}
	r.wg.Add(1)
	r.inflight++
	r.life.Unlock()

	go r.run(context.WithoutCancel(ctx), endpoint, h, req)
	return nil
}

func (r *Registry) run(ctx context.Context, endpoint string, h Handler, req any) {
	defer func() {
		if p := recover(); p != nil {
			r.log.ErrorContext(ctx, "bus publish panic", "endpoint", endpoint, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		}
		r.life.Lock()
		r.inflight--
		r.life.Unlock()
		r.wg.Done()
	}()
	if _, err := h(ctx, req); err != nil {
		r.log.ErrorContext(ctx, "bus publish failed", "endpoint", endpoint, "error", err)
	}
}

// Shutdown menolak Publish baru dan menunggu yang berjalan sampai selesai atau ctx habis.
func (r *Registry) Shutdown(ctx context.Context) error {
	if r == nil {
		return ErrNilRegistry
	}
	r.life.Lock()
	r.closed = true
	n := r.inflight
	r.life.Unlock()
	if n == 0 {
		return nil
	}
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		r.life.Lock()
		n = r.inflight
		r.life.Unlock()
		r.log.Warn("bus shutdown timed out", "inflight", n)
		return fmt.Errorf("bus: shutdown with %d publish still running: %w", n, ctx.Err())
	}
}
