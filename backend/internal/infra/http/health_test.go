package httpx

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHealthz(t *testing.T) {
	s := newStack(t, nil, nil)
	w := do(s.root, "GET", "/healthz?token=secret", nil, "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w.Header().Get(TraceHeader) == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("header trace/keamanan hilang")
	}
	if w := do(s.root, "HEAD", "/healthz", nil, ""); w.Code != 200 {
		t.Fatal("HEAD")
	}
	if strings.Contains(s.logs.String(), "secret") {
		t.Fatal("query rahasia muncul di log")
	}
	w = do(s.root, "GET", "/healthz", map[string]string{TraceHeader: "abc-123"}, "")
	if w.Header().Get(TraceHeader) != "abc-123" {
		t.Fatal("trace id masuk tidak dipakai")
	}
}

func TestHealthz_NotSubjectToAPITimeout(t *testing.T) {
	s := newStack(t, nil, nil)
	s.root.Get("/probe-deadline", func(w http.ResponseWriter, r *http.Request) {
		_, ok := r.Context().Deadline()
		WriteJSON(w, 200, map[string]bool{"deadline": ok})
	})
	if w := do(s.root, "GET", "/probe-deadline", nil, ""); !strings.Contains(w.Body.String(), `false`) {
		t.Fatal(w.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	var sawDeadline time.Duration
	s := newStack(t, nil, fakePinger(func(ctx context.Context) error {
		dl, ok := ctx.Deadline()
		if ok {
			sawDeadline = time.Until(dl)
		}
		return nil
	}))
	w := do(s.root, "GET", "/readyz", nil, "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if sawDeadline <= 0 || sawDeadline > 2*time.Second {
		t.Fatalf("deadline ping %v", sawDeadline)
	}
}

func TestReadyz_503(t *testing.T) {
	s := newStack(t, nil, fakePinger(func(ctx context.Context) error { return context.DeadlineExceeded }))
	w := do(s.root, "GET", "/readyz", nil, "")
	if w.Code != 503 || strings.TrimSpace(w.Body.String()) != `{"status":"unavailable"}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if do(s.root, "GET", "/healthz", nil, "").Code != 200 {
		t.Fatal("healthz harus tetap 200")
	}
	if !strings.Contains(s.logs.String(), "readyz") {
		t.Fatal("error ping harus di-log")
	}
	s2 := newStack(t, nil, nil)
	if do(s2.root, "GET", "/readyz", nil, "").Code != 503 {
		t.Fatal("nil pinger")
	}
}

func TestReadyz_PingTimeoutViaCtx(t *testing.T) {
	s := newStack(t, nil, fakePinger(func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("ctx ping harus punya deadline")
		}
		return context.DeadlineExceeded
	}))
	if do(s.root, "GET", "/readyz", nil, "").Code != 503 {
		t.Fatal("harus 503")
	}
}
