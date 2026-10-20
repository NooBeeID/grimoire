package getentry

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

func newRouter(repo Repository) chi.Router {
	r := chi.NewRouter()
	r.Get("/{id}", NewHandler(NewUsecase(repo)).Get)
	return r
}

func do(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("bukan JSON: %v: %s", err, w.Body.String())
	}
	return m
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	e, _ := decode(t, w)["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func TestGetEntry_InvalidID_404(t *testing.T) {
	f := &fakeRepo{}
	w := do(newRouter(f), http.MethodGet, "/not-a-uuid")
	if w.Code != 404 || errCode(t, w) != "NOT_FOUND" || f.calls != 0 {
		t.Fatalf("code %d %s calls %d", w.Code, w.Body.String(), f.calls)
	}
}

func TestGetEntry_NotFound_404_SameBody(t *testing.T) {
	r := newRouter(&fakeRepo{})
	a := do(r, http.MethodGet, "/not-a-uuid")
	b := do(r, http.MethodGet, "/"+validID)
	if b.Code != 404 || errCode(t, b) != "NOT_FOUND" {
		t.Fatalf("code %d %s", b.Code, b.Body.String())
	}
	ma, mb := decode(t, a), decode(t, b)
	delete(ma, "trace_id")
	delete(mb, "trace_id")
	ja, _ := json.Marshal(ma)
	jb, _ := json.Marshal(mb)
	if string(ja) != string(jb) {
		t.Fatalf("body berbeda: %s vs %s", ja, jb)
	}
}

func TestGetEntry_OK_AllFields(t *testing.T) {
	at := time.Date(2026, 10, 19, 8, 14, 22, 123456000, time.UTC)
	e := base.Entry{
		ID: validID, OccurredAt: at, ActorType: base.ActorAPIKey,
		ActorID: "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", ActorLabel: "claude-code-budi (gk_ab12)",
		ProjectID: "0192f0c4-0000-7000-8000-000000000001", ProjectLabel: "Payments",
		Action: "mcp.read_file", TargetType: "file", Target: "docs/auth/login.md",
		Args:   map[string]any{"path": "docs/auth/login.md", "api_key": "[REDACTED]"},
		Result: map[string]any{"bytes": float64(4812), "content": "[OMITTED]"},
		Status: base.StatusOK, IP: "203.0.113.5", UserAgent: "claude-code/2.1", TraceID: "9c2e41aa07b34f15",
	}
	w := do(newRouter(&fakeRepo{entry: e, found: true}), http.MethodGet, "/"+strings.ToUpper(validID))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("code %d ct %s", w.Code, w.Header().Get("Content-Type"))
	}
	m := decode(t, w)
	if len(m) != 16 {
		t.Fatalf("want 16 kunci, got %d: %v", len(m), m)
	}
	want := map[string]any{
		"id": validID, "occurred_at": "2026-10-19T08:14:22.123456Z", "actor_type": "api_key",
		"actor_id": e.ActorID, "actor_label": e.ActorLabel, "project_id": e.ProjectID,
		"project_label": "Payments", "action": "mcp.read_file", "target_type": "file",
		"target": "docs/auth/login.md", "status": "ok", "ip": "203.0.113.5",
		"user_agent": "claude-code/2.1", "trace_id": "9c2e41aa07b34f15",
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s: got %v want %v", k, m[k], v)
		}
	}
	if m["args"].(map[string]any)["api_key"] != "[REDACTED]" || m["result"].(map[string]any)["content"] != "[OMITTED]" {
		t.Errorf("args/result salah: %v %v", m["args"], m["result"])
	}
}

func TestGetEntry_NullableFields(t *testing.T) {
	e := base.Entry{
		ID: validID, OccurredAt: time.Now(), ActorType: base.ActorAnonymous, ActorLabel: "gk_zz99",
		Action: "mcp.auth.rejected", Status: base.StatusDenied, IP: "198.51.100.7", TraceID: "t",
		Result: map[string]any{"reason": "invalid_key"},
	}
	w := do(newRouter(&fakeRepo{entry: e, found: true}), http.MethodGet, "/"+validID)
	m := decode(t, w)
	for _, k := range []string{"actor_id", "project_id", "project_label", "target_type", "target", "user_agent"} {
		if v, ok := m[k]; !ok || v != nil {
			t.Errorf("%s harus null, got %v (ada=%v)", k, v, ok)
		}
	}
	if m["ip"] != "198.51.100.7" {
		t.Errorf("ip: %v", m["ip"])
	}
	if a, ok := m["args"].(map[string]any); !ok || len(a) != 0 {
		t.Errorf("args harus {}: %v", m["args"])
	}
}

func TestGetEntry_InternalError_500(t *testing.T) {
	w := do(newRouter(&fakeRepo{err: errors.New("secret sql failure")}), http.MethodGet, "/"+validID)
	if w.Code != 500 || errCode(t, w) != "INTERNAL" || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
}

func TestGetEntry_OnlyGetRegistered(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/audit-log", func(r chi.Router) { Init(testDeps(), r) })
	var routes []string
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, method+" "+route)
		return nil
	})
	if len(routes) != 1 || routes[0] != "GET /audit-log/{id}" {
		t.Fatalf("rute: %v", routes)
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if w := do(r, m, "/audit-log/"+validID); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, w.Code)
		}
	}
}
