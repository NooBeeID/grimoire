package listentries

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

type fakeUC struct {
	called int
	res    Result
	err    error
}

func (f *fakeUC) List(context.Context, base.Filter) (Result, error) {
	f.called++
	return f.res, f.err
}

func get(uc lister, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/", NewHandler(uc).List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func errBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	e, _ := m["error"].(map[string]any)
	return e
}

func TestHandler_InvalidFilter(t *testing.T) {
	for target, field := range map[string]string{
		"/?limit=500":     "limit",
		"/?cursor=rusak!": "cursor",
		"/?from=bukan":    "from",
		"/?status=x":      "status",
		"/?actor_id=abc":  "actor_id",
	} {
		uc := &fakeUC{}
		w := get(uc, target)
		if w.Code != http.StatusBadRequest || uc.called != 0 {
			t.Fatalf("%s: code=%d called=%d", target, w.Code, uc.called)
		}
		e := errBody(t, w)
		d, _ := e["details"].(map[string]any)
		if e["code"] != "INVALID_ARGUMENT" || d["field"] != field {
			t.Fatalf("%s: %v", target, e)
		}
	}
}

func TestHandler_InternalErrorGeneric(t *testing.T) {
	w := get(&fakeUC{err: errors.New("pq: relation secret_table")}, "/")
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "secret_table") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if errBody(t, w)["code"] != "INTERNAL" {
		t.Fatal(w.Body.String())
	}
}

func TestHandler_Success(t *testing.T) {
	w := get(&fakeUC{res: Result{Items: []base.Summary{}}}, "/")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %s", w.Code, w.Header())
	}
	if strings.TrimSpace(w.Body.String()) != `{"items":[],"next_cursor":null}` {
		t.Fatal(w.Body.String())
	}
}
