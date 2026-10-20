package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"grimoire/lib/common/reqctx"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func testLimiter(t *testing.T, rule Rule, clk *fakeClock) *Limiter {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	l := newLimiter(ctx, rule, clk.Now, time.Hour, 10*time.Minute)
	t.Cleanup(func() { cancel(); <-l.done })
	return l
}

func TestRateLimit_429WithRetryAfter(t *testing.T) {
	useLogger(t)
	clk := &fakeClock{t: time.Unix(1000, 0)}
	l := testLimiter(t, Rule{Name: "t", Rate: 0.5, Burst: 2, KeyFunc: KeyByIP}, clk)
	h := RateLimit(l)(http.HandlerFunc(okHandler))
	for i := 0; i < 2; i++ {
		if w := do(h, "GET", "/", nil, ""); w.Code != 200 {
			t.Fatalf("req %d: %d", i, w.Code)
		}
	}
	w := do(h, "GET", "/", nil, "")
	e := decodeErr(t, w)
	ra, err := strconv.Atoi(w.Header().Get("Retry-After"))
	d, _ := e.Error.Details.(map[string]any)
	if w.Code != 429 || e.Error.Code != "RATE_LIMITED" || err != nil || ra < 1 || d["retry_after_seconds"] != float64(ra) {
		t.Fatalf("%d %+v ra=%q", w.Code, e, w.Header().Get("Retry-After"))
	}
	// ditolak tidak menghabiskan token: setelah 2 detik (rate 0.5/s) 1 token tersedia
	clk.Add(2 * time.Second)
	if w := do(h, "GET", "/", nil, ""); w.Code != 200 {
		t.Fatalf("setelah refill: %d", w.Code)
	}
	if w := do(h, "GET", "/", nil, ""); w.Code != 429 {
		t.Fatalf("token habis lagi: %d", w.Code)
	}
}

func TestRateLimit_KeysIndependentAndEmptyKey(t *testing.T) {
	useLogger(t)
	clk := &fakeClock{t: time.Unix(1000, 0)}
	key := func(r *http.Request) string { return r.Header.Get("K") }
	l := testLimiter(t, Rule{Rate: 0.001, Burst: 1, KeyFunc: key}, clk)
	h := RateLimit(l)(http.HandlerFunc(okHandler))
	hit := func(k string) int { return do(h, "GET", "/", map[string]string{"K": k}, "").Code }
	if hit("a") != 200 || hit("a") != 429 || hit("b") != 200 {
		t.Fatal("key tidak independen")
	}
	for i := 0; i < 5; i++ {
		if hit("") != 200 {
			t.Fatal("key kosong tidak dibatasi")
		}
	}
}

func TestKeyByUser(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "9.9.9.9:1"
	if KeyByUser(r) != "9.9.9.9" {
		t.Fatal("fallback IP")
	}
	r = r.WithContext(reqctx.WithUserID(r.Context(), "u-1"))
	if KeyByUser(r) != "u-1" {
		t.Fatal("user")
	}
}

func TestLimiter_JanitorEvictsIdle(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	l := testLimiter(t, Rule{Rate: 1, Burst: 1, KeyFunc: KeyByIP}, clk)
	l.Allow("a")
	clk.Add(5 * time.Minute)
	l.Allow("b")
	clk.Add(6 * time.Minute) // a idle 11m, b idle 6m
	l.sweep()
	if l.size() != 1 {
		t.Fatalf("size=%d", l.size())
	}
	l.mu.Lock()
	_, hasB := l.entries["b"]
	l.mu.Unlock()
	if !hasB {
		t.Fatal("b harus tersisa")
	}
}

func TestLimiter_JanitorStopsOnCtxCancelAndTicks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clk := &fakeClock{t: time.Unix(1000, 0)}
	l := newLimiter(ctx, Rule{Rate: 1, Burst: 1, KeyFunc: KeyByIP}, clk.Now, 5*time.Millisecond, time.Minute)
	l.Allow("a")
	clk.Add(2 * time.Minute)
	deadline := time.After(2 * time.Second)
	for l.size() != 0 {
		select {
		case <-deadline:
			t.Fatal("janitor tidak membuang entri idle")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-l.done:
	case <-time.After(2 * time.Second):
		t.Fatal("janitor tidak berhenti")
	}
}

func TestNewLimiter_InvalidRulePanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustPanic(t, func() { NewLimiter(ctx, Rule{Rate: 0, Burst: 1, KeyFunc: KeyByIP}) })
	mustPanic(t, func() { NewLimiter(ctx, Rule{Rate: 1, Burst: 0, KeyFunc: KeyByIP}) })
	mustPanic(t, func() { NewLimiter(ctx, Rule{Rate: 1, Burst: 1}) })
}
