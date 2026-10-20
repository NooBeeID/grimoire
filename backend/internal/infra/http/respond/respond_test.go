package respond

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

func TestWriteError_Mapping(t *testing.T) {
	useLogger(t)
	cases := []struct {
		name   string
		err    error
		status int
		code   errs.Code
		msg    string
	}{
		{"errs.Error", errs.New(errs.CodeConflict, "bentrok"), 409, errs.CodeConflict, "bentrok"},
		{"wrapped errs", fmt.Errorf("x: %w", errs.New(errs.CodeForbidden, "no")), 403, errs.CodeForbidden, "no"},
		{"not found", errs.ErrNotFound, 404, errs.CodeNotFound, "resource tidak ditemukan"},
		{"forbidden", fmt.Errorf("w: %w", errs.ErrForbidden), 403, errs.CodeForbidden, "akses ditolak"},
		{"conflict", errs.ErrConflict, 409, errs.CodeConflict, "konflik data"},
		{"invalid", errs.ErrInvalid, 400, errs.CodeInvalid, "permintaan tidak valid"},
		{"unauthorized", errs.ErrUnauthorized, 401, errs.CodeUnauthorized, "perlu login"},
		{"deadline", context.DeadlineExceeded, 504, errs.CodeTimeout, "permintaan melebihi batas waktu"},
		{"unknown", errors.New("pq: connection refused host=10.0.0.5"), 500, errs.CodeInternal, genericInternal},
		{"canceled", context.Canceled, 500, errs.CodeInternal, genericInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			WriteError(w, r, c.err)
			e := decodeErr(t, w)
			if w.Code != c.status || string(e.Error.Code) != string(c.code) || e.Error.Message != c.msg {
				t.Fatalf("got %d %s %q", w.Code, e.Error.Code, e.Error.Message)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Fatalf("content-type %q", ct)
			}
		})
	}
}

func TestWriteError_NoLeakAndLogged(t *testing.T) {
	_, buf := useLogger(t)
	r := httptest.NewRequest("GET", "/", nil).WithContext(reqctx.WithTraceID(context.Background(), "tid-1"))
	w := httptest.NewRecorder()
	WriteError(w, r, errors.New("pq: connection refused host=10.0.0.5"))
	body := w.Body.String()
	if strings.Contains(body, "pq") || strings.Contains(body, "10.0.0.5") {
		t.Fatalf("bocor: %s", body)
	}
	logs := buf.String()
	if !strings.Contains(logs, "10.0.0.5") || !strings.Contains(logs, "tid-1") {
		t.Fatalf("log tidak lengkap: %s", logs)
	}
}

func TestWriteError_InternalCodeMessageForcedGeneric(t *testing.T) {
	useLogger(t)
	w := httptest.NewRecorder()
	WriteError(w, httptest.NewRequest("GET", "/", nil), errs.WithDetails(errs.CodeInternal, "rahasia", "d"))
	e := decodeErr(t, w)
	if e.Error.Message != genericInternal || e.Error.Details != nil {
		t.Fatalf("%+v", e)
	}
}

func TestWriteError_DetailsNullAndTraceFallback(t *testing.T) {
	useLogger(t)
	w := httptest.NewRecorder()
	w.Header().Set(TraceHeader, "from-header")
	WriteError(w, httptest.NewRequest("GET", "/", nil), errs.ErrNotFound)
	if !strings.Contains(w.Body.String(), `"details":null`) {
		t.Fatal(w.Body.String())
	}
	if decodeErr(t, w).TraceID != "from-header" {
		t.Fatal("fallback header gagal")
	}
	w = httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil).WithContext(reqctx.WithTraceID(context.Background(), "from-ctx"))
	WriteError(w, r, errs.ErrNotFound)
	if decodeErr(t, w).TraceID != "from-ctx" {
		t.Fatal("trace ctx gagal")
	}
}

func TestWriteError_MaxBytes(t *testing.T) {
	useLogger(t)
	w := httptest.NewRecorder()
	WriteError(w, httptest.NewRequest("GET", "/", nil), fmt.Errorf("w: %w", &http.MaxBytesError{Limit: 123}))
	e := decodeErr(t, w)
	d, _ := e.Error.Details.(map[string]any)
	if w.Code != 413 || e.Error.Code != "PAYLOAD_TOO_LARGE" || d["limit_bytes"] != float64(123) {
		t.Fatalf("%d %+v", w.Code, e)
	}
}

func TestWriteJSON_NoBodyFor204(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, 204, map[string]int{"a": 1})
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal("204 harus tanpa body")
	}
}

func TestDecodeJSON(t *testing.T) {
	type in struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	cases := []struct {
		name, body, msg, field string
		ok                     bool
	}{
		{"valid", `{"name":"a","age":3}`, "", "", true},
		{"kosong", ``, "body wajib diisi", "", false},
		{"sintaks", `{"name":`, "JSON tidak valid", "", false},
		{"sintaks2", `{name}`, "JSON tidak valid", "", false},
		{"unknown", `{"nama":"a"}`, "field tidak dikenal", "nama", false},
		{"tipe", `{"age":"x"}`, "tipe field tidak sesuai", "age", false},
		{"sisa", `{"name":"a"} {"name":"b"}`, "JSON tidak valid", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(c.body))
			var v in
			err := DecodeJSON(r, &v)
			if c.ok {
				if err != nil || v.Name != "a" {
					t.Fatalf("err=%v v=%+v", err, v)
				}
				return
			}
			var e *errs.Error
			if !errors.As(err, &e) || e.Code != errs.CodeInvalid || e.Message != c.msg {
				t.Fatalf("err=%v", err)
			}
			if c.field != "" {
				if m, _ := e.Details.(map[string]string); m["field"] != c.field {
					t.Fatalf("details=%v", e.Details)
				}
			}
		})
	}
}

func TestDecodeJSON_TooLarge(t *testing.T) {
	useLogger(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"`+strings.Repeat("a", 100)+`"}`))
	r.Body = http.MaxBytesReader(w, r.Body, 20)
	var v struct{ Name string }
	err := DecodeJSON(r, &v)
	WriteError(w, r, err)
	e := decodeErr(t, w)
	if w.Code != 413 || e.Error.Code != "PAYLOAD_TOO_LARGE" {
		t.Fatalf("%d %+v err=%v", w.Code, e, err)
	}
}
