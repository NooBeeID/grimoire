//go:build integration

package summary

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/resolvesession"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

func withIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role := r.Header.Get("X-Test-Role"); role != "" {
			r = r.WithContext(reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: uuid.NewString(), Role: role}))
		}
		next.ServeHTTP(w, r)
	})
}

type env struct {
	h     http.Handler
	calls *int
	d     app.Deps
}

// qualityMode: "ok", "err", atau "none" (tidak terdaftar).
func newEnv(t *testing.T, mode string) *env {
	t.Helper()
	d := setupDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := bus.NewRegistry(log)
	calls := 0
	switch mode {
	case "ok":
		b.Register(constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, func(_ context.Context, req any) (any, error) {
			calls++
			r := req.(searchquality.Request)
			return searchquality.Response{Report: searchquality.Report{
				ProjectID: r.ProjectID, Since: r.Since, Until: r.Until, WindowSeconds: 120, Searches: 7,
				ZeroResult: searchquality.Metric{Count: 1, Denominator: 7, Rate: 1.0 / 7},
				RankOpened: []searchquality.RankBucket{{Label: "1", Count: 2}},
			}}, nil
		})
	case "err":
		b.Register(constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, func(context.Context, any) (any, error) {
			calls++
			return nil, errors.New("T11 down")
		})
	}
	deps := app.Deps{Ctx: context.Background(), Log: log, DB: d, Bus: b}
	r := chi.NewRouter()
	r.Use(withIdentity)
	r.Route("/api/audit-log", func(r chi.Router) {
		r.Use(middleware.RequireRole(resolvesession.RoleSuperAdmin))
		Init(deps, r)
		r.Get("/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	})
	return &env{h: r, calls: &calls, d: deps}
}

func (e *env) do(method, target, role string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if role != "" {
		req.Header.Set("X-Test-Role", role)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

const sa = resolvesession.RoleSuperAdmin

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return m
}

func TestT15_HTTP_Authorization(t *testing.T) {
	e := newEnv(t, "none")
	if c := e.do("GET", "/api/audit-log/summary", "").Code; c != 401 {
		t.Fatalf("anon %d", c)
	}
	if c := e.do("GET", "/api/audit-log/summary", "developer").Code; c != 403 {
		t.Fatalf("developer %d", c)
	}
	if c := e.do("GET", "/api/audit-log/summary", sa).Code; c != 200 {
		t.Fatalf("super_admin %d", c)
	}
}

func TestT15_HTTP_RouteNotShadowedByID(t *testing.T) {
	e := newEnv(t, "none")
	if c := e.do("GET", "/api/audit-log/summary", sa).Code; c == http.StatusTeapot {
		t.Fatal("/summary jatuh ke /{id}")
	}
	for _, m := range []string{"POST", "PUT", "DELETE"} {
		if c := e.do(m, "/api/audit-log/summary", sa).Code; c != 405 {
			t.Fatalf("%s: %d", m, c)
		}
	}
}

func TestT15_AC7_Validation(t *testing.T) {
	e := newEnv(t, "ok")
	for _, q := range []string{"range=1d", "project_id=xyz"} {
		if c := e.do("GET", "/api/audit-log/summary?"+q, sa).Code; c != 400 {
			t.Fatalf("%s: %d", q, c)
		}
	}
}

func TestT15_AC7_EmptyProject(t *testing.T) {
	e := newEnv(t, "ok")
	w := e.do("GET", "/api/audit-log/summary?project_id="+uuid.NewString(), sa)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	m := decode(t, w)
	if m["total_calls"].(float64) != 0 || m["error_rate"] != nil {
		t.Fatalf("%v", m)
	}
	for _, k := range []string{"top_projects", "tool_usage", "tool_metrics"} {
		if a, ok := m[k].([]any); !ok || len(a) != 0 {
			t.Fatalf("%s = %v", k, m[k])
		}
	}
	tr := m["traffic"].([]any)
	if len(tr) < 7 {
		t.Fatalf("traffic %d", len(tr))
	}
	for _, p := range tr {
		if p.(map[string]any)["calls"].(float64) != 0 {
			t.Fatal("traffic tidak nol")
		}
	}
	if sq := m["search_quality"].(map[string]any); sq["available"] != true {
		t.Fatalf("sq %v", sq)
	}
}

func TestT15_AC8_QualityShape(t *testing.T) {
	e := newEnv(t, "ok")
	m := decode(t, e.do("GET", "/api/audit-log/summary?range=24h&project_id="+uuid.NewString(), sa))
	sq := m["search_quality"].(map[string]any)
	if sq["available"] != true || sq["searches"].(float64) != 7 || sq["window_seconds"].(float64) != 120 {
		t.Fatalf("sq %v", sq)
	}
	if *e.calls != 1 {
		t.Fatalf("calls %d", *e.calls)
	}
	m = decode(t, e.do("GET", "/api/audit-log/summary", sa))
	if v, ok := m["search_quality"]; !ok || v != nil {
		t.Fatalf("tanpa project harus null: %v", v)
	}
	if *e.calls != 1 {
		t.Fatalf("bus dipanggil tanpa project: %d", *e.calls)
	}
}

func TestT15_AC9_QualityFailSoft(t *testing.T) {
	for _, mode := range []string{"err", "none"} {
		e := newEnv(t, mode)
		w := e.do("GET", "/api/audit-log/summary?project_id="+uuid.NewString(), sa)
		if w.Code != 200 {
			t.Fatalf("%s: %d", mode, w.Code)
		}
		m := decode(t, w)
		b, _ := json.Marshal(m["search_quality"])
		if string(b) != `{"available":false,"error_code":"UNAVAILABLE"}` {
			t.Fatalf("%s: %s", mode, b)
		}
		if m["traffic"] == nil || m["total_calls"] == nil {
			t.Fatalf("%s: field lain hilang", mode)
		}
	}
}

func TestT15_AC10_NoSensitiveDataAndNoAudit(t *testing.T) {
	e := newEnv(t, "none")
	pid := uuid.NewString()
	_, err := e.d.DB.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_label, project_id, project_label, action, target, status, args, result, ip, user_agent)
		VALUES ($1, now(), 'api_key','key',$2::uuid,'Payments','mcp.read_file','docs/secret.md','ok',
		 '{"query":"rahasia-query","path":"docs/secret.md"}'::jsonb,'{"result_paths":["docs/secret.md"],"duration_ms":5}'::jsonb,'10.9.8.7','agent-ua-xyz')`, uuid.NewString(), pid)
	if err != nil {
		t.Fatal(err)
	}
	var before, after int
	_ = e.d.DB.QueryRow(`SELECT count(*) FROM audit_log`).Scan(&before)
	w := e.do("GET", "/api/audit-log/summary?project_id="+pid, sa)
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	body := w.Body.String()
	for _, s := range []string{"rahasia-query", "docs/secret.md", "result_paths", "10.9.8.7", "agent-ua-xyz"} {
		if strings.Contains(body, s) {
			t.Fatalf("bocor %q: %s", s, body)
		}
	}
	banned := map[string]bool{"args": true, "result": true, "target": true, "query": true, "path": true, "ip": true, "user_agent": true, "trace_id": true}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				if banned[k] {
					t.Fatalf("kunci terlarang %q", k)
				}
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(decode(t, w))
	_ = e.d.DB.QueryRow(`SELECT count(*) FROM audit_log`).Scan(&after)
	if before != after {
		t.Fatalf("audit_log berubah %d -> %d", before, after)
	}
}
