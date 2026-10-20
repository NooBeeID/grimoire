package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"
	"strings"
	"testing"
)

func TestRecover_PanicReturns500JSON(t *testing.T) {
	log, buf := useLogger(t)
	h := TraceID()(Recover(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("boom-rahasia") })))
	w := do(h, "GET", "/", map[string]string{respond.TraceHeader: "abc-1"}, "")
	e := decodeErr(t, w)
	if w.Code != 500 || e.Error.Message != "terjadi kesalahan internal" || e.TraceID != "abc-1" {
		t.Fatalf("%d %+v", w.Code, e)
	}
	if strings.Contains(w.Body.String(), "boom") || strings.Contains(w.Body.String(), "goroutine") {
		t.Fatal("body bocor")
	}
	rec := buf.find(t, "panic recovered")
	if rec == nil || rec["stack"] == "" || rec["trace_id"] != "abc-1" {
		t.Fatalf("log: %v", rec)
	}
}

func TestRecover_AfterHeaderSentOnlyLogs(t *testing.T) {
	log, buf := useLogger(t)
	h := Recover(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		panic("late")
	}))
	w := do(h, "GET", "/", nil, "")
	if w.Code != 201 || w.Body.Len() != 0 {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	if buf.find(t, "panic recovered") == nil {
		t.Fatal("harus di-log")
	}
}

func TestRecover_ErrAbortHandlerRepanics(t *testing.T) {
	log, _ := useLogger(t)
	h := Recover(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }))
	defer func() {
		if recover() != http.ErrAbortHandler {
			t.Fatal("harus panic ulang")
		}
	}()
	do(h, "GET", "/", nil, "")
}
