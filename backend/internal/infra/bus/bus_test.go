package bus

import (
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

type rec struct {
	slog.Record
	trace string
	user  string
}

type captureHandler struct {
	mu   *sync.Mutex
	recs *[]rec
}

func (h captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h captureHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.recs = append(*h.recs, rec{Record: r.Clone(), trace: reqctx.TraceID(ctx), user: reqctx.UserID(ctx)})
	return nil
}
func (h captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h captureHandler) WithGroup(string) slog.Handler      { return h }

type capture struct {
	mu   sync.Mutex
	recs []rec
}

func (c *capture) logger() *slog.Logger {
	return slog.New(captureHandler{mu: &c.mu, recs: &c.recs})
}

func (c *capture) find(msg string) (rec, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.recs {
		if r.Message == msg {
			return r, true
		}
	}
	return rec{}, false
}

func attrs(r rec) map[string]any {
	m := map[string]any{}
	r.Attrs(func(a slog.Attr) bool { m[a.Key] = a.Value.Any(); return true })
	return m
}

func wait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
		panic("unreachable")
	}
}

func mustPanic(t *testing.T, f func()) string {
	t.Helper()
	var msg string
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("expected panic")
			}
			msg, _ = p.(string)
		}()
		f()
	}()
	return msg
}

func noop(context.Context, any) (any, error) { return nil, nil }

func TestT00_AC3_RegisterDuplicatePanics(t *testing.T) {
	r := NewRegistry(nil)
	r.Register("a.b.c", noop)
	msg := mustPanic(t, func() { r.Register("a.b.c", noop) })
	if !strings.Contains(msg, "registered twice") || !strings.Contains(msg, "a.b.c") {
		t.Fatalf("msg=%q", msg)
	}
}

func TestRegister_NilHandlerOrEmptyEndpointPanics(t *testing.T) {
	r := NewRegistry(nil)
	mustPanic(t, func() { r.Register("a.b.c", nil) })
	mustPanic(t, func() { r.Register("", noop) })
}

func TestT00_AC3_RegisterOrderIndependent(t *testing.T) {
	ha := func(context.Context, any) (any, error) { return "A", nil }
	hb := func(context.Context, any) (any, error) { return "B", nil }
	r1, r2 := NewRegistry(nil), NewRegistry(nil)
	r1.Register("x.y.a", ha)
	r1.Register("x.y.b", hb)
	r2.Register("x.y.b", hb)
	r2.Register("x.y.a", ha)
	for _, r := range []*Registry{r1, r2} {
		a, _ := r.Call(context.Background(), "x.y.a", nil)
		b, _ := r.Call(context.Background(), "x.y.b", nil)
		if a != "A" || b != "B" {
			t.Fatalf("a=%v b=%v", a, b)
		}
	}
}

func TestT00_AC3_CallUnregistered(t *testing.T) {
	res, err := NewRegistry(nil).Call(context.Background(), "x.y.z", nil)
	if !errors.Is(err, ErrNotRegistered) || !strings.Contains(err.Error(), "x.y.z") || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCall_PassesThroughResponseAndError(t *testing.T) {
	sentinel := errors.New("boom")
	r := NewRegistry(nil)
	r.Register("x.y.ok", func(context.Context, any) (any, error) { return 42, nil })
	r.Register("x.y.err", func(context.Context, any) (any, error) { return nil, sentinel })
	if v, err := r.Call(context.Background(), "x.y.ok", nil); err != nil || v != 42 {
		t.Fatalf("v=%v err=%v", v, err)
	}
	if _, err := r.Call(context.Background(), "x.y.err", nil); err != sentinel {
		t.Fatalf("err=%v", err)
	}
}

func TestCall_UsesCallerContext(t *testing.T) {
	r := NewRegistry(nil)
	r.Register("x.y.z", func(ctx context.Context, _ any) (any, error) {
		return reqctx.TraceID(ctx), ctx.Err()
	})
	ctx, cancel := context.WithCancel(reqctx.WithTraceID(context.Background(), "t1"))
	v, err := r.Call(ctx, "x.y.z", nil)
	if err != nil || v != "t1" {
		t.Fatalf("v=%v err=%v", v, err)
	}
	cancel()
	if _, err := r.Call(ctx, "x.y.z", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestCall_PanicPropagates(t *testing.T) {
	r := NewRegistry(nil)
	r.Register("x.y.z", func(context.Context, any) (any, error) { panic("kaboom") })
	mustPanic(t, func() { _, _ = r.Call(context.Background(), "x.y.z", nil) })
}

func TestT00_AC3_PublishCarriesContextValues(t *testing.T) {
	r := NewRegistry(nil)
	ch := make(chan [3]string, 1)
	r.Register("x.y.z", func(ctx context.Context, _ any) (any, error) {
		ch <- [3]string{reqctx.TraceID(ctx), reqctx.UserID(ctx), reqctx.APIKeyID(ctx)}
		return nil, nil
	})
	ctx := reqctx.WithAPIKeyID(reqctx.WithUserID(reqctx.WithTraceID(context.Background(), "t"), "u"), "k")
	if err := r.Publish(ctx, "x.y.z", nil); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, ch); got != [3]string{"t", "u", "k"} {
		t.Fatalf("got=%v", got)
	}
}

func TestT00_AC3_PublishSurvivesParentCancel(t *testing.T) {
	r := NewRegistry(nil)
	release := make(chan struct{})
	res := make(chan bool, 1)
	r.Register("x.y.z", func(ctx context.Context, _ any) (any, error) {
		<-release
		res <- ctx.Err() == nil && ctx.Done() == nil
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := r.Publish(ctx, "x.y.z", nil); err != nil {
		t.Fatal(err)
	}
	cancel()
	close(release)
	if !wait(t, res) {
		t.Fatal("ctx was canceled in publish handler")
	}
}

func TestT00_AC3_PublishPanicRecovered(t *testing.T) {
	var c capture
	r := NewRegistry(c.logger())
	r.Register("x.y.boom", func(context.Context, any) (any, error) { panic("boom") })
	ch := make(chan struct{}, 1)
	r.Register("x.y.next", func(context.Context, any) (any, error) { ch <- struct{}{}; return nil, nil })
	if err := r.Publish(context.Background(), "x.y.boom", nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	rc, ok := c.find("bus publish panic")
	if !ok {
		t.Fatal("no panic log")
	}
	a := attrs(rc)
	if rc.Level != slog.LevelError || a["endpoint"] != "x.y.boom" || a["panic"] != "boom" || a["stack"] == "" {
		t.Fatalf("attrs=%v", a)
	}
	// registry sudah closed oleh Shutdown; verifikasi publish lain di registry baru tetap jalan
	r2 := NewRegistry(c.logger())
	r2.Register("x.y.boom", func(context.Context, any) (any, error) { panic("boom") })
	r2.Register("x.y.next", func(context.Context, any) (any, error) { ch <- struct{}{}; return nil, nil })
	_ = r2.Publish(context.Background(), "x.y.boom", nil)
	_ = r2.Publish(context.Background(), "x.y.next", nil)
	wait(t, ch)
}

func TestPublish_ErrorLogged(t *testing.T) {
	var c capture
	r := NewRegistry(c.logger())
	r.Register("x.y.z", func(context.Context, any) (any, error) { return nil, errors.New("gagal") })
	ctx := reqctx.WithTraceID(context.Background(), "tr")
	if err := r.Publish(ctx, "x.y.z", nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	rc, ok := c.find("bus publish failed")
	if !ok {
		t.Fatal("no log")
	}
	a := attrs(rc)
	if a["endpoint"] != "x.y.z" || a["error"] == nil || rc.trace != "tr" {
		t.Fatalf("attrs=%v trace=%q", a, rc.trace)
	}
}

func TestPublish_UnregisteredReturnsError(t *testing.T) {
	r := NewRegistry(nil)
	if err := r.Publish(context.Background(), "x.y.z", nil); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("err=%v", err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func blocker(r *Registry, endpoint string) (release func()) {
	started := make(chan struct{})
	rel := make(chan struct{})
	r.Register(endpoint, func(context.Context, any) (any, error) {
		close(started)
		<-rel
		return nil, nil
	})
	_ = r.Publish(context.Background(), endpoint, nil)
	<-started
	var once sync.Once
	return func() { once.Do(func() { close(rel) }) }
}

func TestShutdown_WaitsForInflight(t *testing.T) {
	r := NewRegistry(nil)
	release := blocker(r, "x.y.z")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Shutdown(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	if err := wait(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestShutdown_TimesOut(t *testing.T) {
	var c capture
	r := NewRegistry(c.logger())
	release := blocker(r, "x.y.z")
	t.Cleanup(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := r.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	rc, ok := c.find("bus shutdown timed out")
	if !ok || attrs(rc)["inflight"] != int64(1) {
		t.Fatalf("log=%v ok=%v", rc, ok)
	}
}

func TestT00_AC3_PublishAfterShutdownRejected(t *testing.T) {
	r := NewRegistry(nil)
	var called atomic.Bool
	r.Register("x.y.z", func(context.Context, any) (any, error) { called.Store(true); return nil, nil })
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Publish(context.Background(), "x.y.z", nil); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("err=%v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if called.Load() {
		t.Fatal("handler called")
	}
}

func TestShutdown_Idempotent(t *testing.T) {
	r := NewRegistry(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 2; i++ {
		if err := r.Shutdown(ctx); err != nil {
			t.Fatalf("i=%d err=%v", i, err)
		}
	}
}

func TestShutdown_NoInflightReturnsImmediately(t *testing.T) {
	start := time.Now()
	if err := NewRegistry(nil).Shutdown(context.Background()); err != nil || time.Since(start) > 100*time.Millisecond {
		t.Fatalf("err=%v", err)
	}
}

func TestConcurrent_RegisterCallPublish(t *testing.T) {
	r := NewRegistry(nil)
	var cnt atomic.Int64
	r.Register("x.y.pub", func(context.Context, any) (any, error) { cnt.Add(1); return nil, nil })
	r.Register("x.y.call", func(context.Context, any) (any, error) { return 1, nil })
	var ok atomic.Int64
	var wg sync.WaitGroup
	names := []string{"a", "b", "c", "d", "e"}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := r.Call(context.Background(), "x.y.call", nil); err != nil {
				t.Error(err)
			}
			if err := r.Publish(context.Background(), "x.y.pub", nil); err == nil {
				ok.Add(1)
			}
			if i < len(names) {
				r.Register("x.z."+names[i], noop)
			}
		}(i)
	}
	wg.Wait()
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cnt.Load() != ok.Load() || ok.Load() != 50 {
		t.Fatalf("cnt=%d ok=%d", cnt.Load(), ok.Load())
	}
}

func TestConcurrent_PublishVsShutdown(t *testing.T) {
	for iter := 0; iter < 20; iter++ {
		r := NewRegistry(nil)
		var ran, ok atomic.Int64
		r.Register("x.y.z", func(context.Context, any) (any, error) {
			time.Sleep(time.Millisecond)
			ran.Add(1)
			return nil, nil
		})
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if r.Publish(context.Background(), "x.y.z", nil) == nil {
					ok.Add(1)
				}
			}()
		}
		if err := r.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		// publish yang sukses sebelum closed pasti sudah selesai
		wg.Wait()
		if err := r.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		if ran.Load() != ok.Load() {
			t.Fatalf("ran=%d ok=%d", ran.Load(), ok.Load())
		}
	}
}

func TestNilRegistry_CallAndPublish(t *testing.T) {
	var r *Registry
	if _, err := r.Call(context.Background(), "x.y.z", nil); !errors.Is(err, ErrNilRegistry) {
		t.Fatal(err)
	}
	if err := r.Publish(context.Background(), "x.y.z", nil); !errors.Is(err, ErrNilRegistry) {
		t.Fatal(err)
	}
}
