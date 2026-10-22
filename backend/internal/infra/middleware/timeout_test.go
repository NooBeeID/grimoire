package middleware

import (
	"context"
	"grimoire/internal/infra/http/respond"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeout(t *testing.T) {
	useLogger(t)
	h := Timeout(20 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		respond.WriteError(w, r, r.Context().Err())
	}))
	w := do(h, "GET", "/", nil, "")
	if w.Code != 504 || decodeErr(t, w).Error.Code != "TIMEOUT" {
		t.Fatal(w.Code)
	}
	var dl time.Time
	override := Timeout(time.Hour)(Timeout(time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl, _ = r.Context().Deadline()
	})))
	do(override, "GET", "/", nil, "")
	if time.Until(dl) > time.Minute {
		t.Fatal("deadline terdalam harus menang")
	}
	mustPanic(t, func() { Timeout(0) })
}

type ctxKeyT struct{}

func TestExtendTimeout(t *testing.T) {
	useLogger(t)
	var dl time.Time
	var val any
	inner := ExtendTimeout(time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl, _ = r.Context().Deadline()
		val = r.Context().Value(ctxKeyT{})
	}))
	setVal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyT{}, "identity")))
	})
	do(Timeout(time.Second)(setVal), "GET", "/", nil, "")
	if time.Until(dl) < 50*time.Minute {
		t.Fatalf("deadline harus diperpanjang, sisa %s", time.Until(dl))
	}
	if val != "identity" {
		t.Fatal("nilai ctx hilang")
	}
	// tanpa Timeout di depan
	dl = time.Time{}
	do(inner, "GET", "/", nil, "")
	if time.Until(dl) < 50*time.Minute {
		t.Fatal("tanpa Timeout harus tetap berfungsi")
	}
	mustPanic(t, func() { ExtendTimeout(0) })
}

func TestExtendTimeout_CancelPropagates(t *testing.T) {
	useLogger(t)
	parent, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	h := Timeout(time.Second)(ExtendTimeout(time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		select {
		case <-r.Context().Done():
			done <- r.Context().Err()
		case <-time.After(time.Second):
			done <- nil
		}
	})))
	req := httptest.NewRequest("GET", "/", nil).WithContext(parent)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if err := <-done; err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}
