//go:build integration

package usage

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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/resolvesession"
	"grimoire/lib/contracts/project/knowledge/listfiles"
	"grimoire/lib/contracts/project/management/getproject"
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
	h http.Handler
	d app.Deps
}

// newEnv: project "known" ada; list-files mengembalikan files.
func newEnv(t *testing.T, known string, files []listfiles.Item, listErr error) *env {
	t.Helper()
	d := setupDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := bus.NewRegistry(log)
	b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(_ context.Context, req any) (any, error) {
		return getproject.Response{Found: req.(getproject.Request).ProjectID == known}, nil
	})
	b.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, func(context.Context, any) (any, error) {
		if listErr != nil {
			return nil, listErr
		}
		return listfiles.Response{Items: files, Total: len(files)}, nil
	})
	deps := app.Deps{Ctx: context.Background(), Log: log, DB: d, Bus: b}
	r := chi.NewRouter()
	r.Use(withIdentity)
	r.Route("/api/audit-log", func(r chi.Router) {
		r.Use(middleware.RequireRole(resolvesession.RoleSuperAdmin))
		Init(deps, r)
		r.Get("/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	})
	return &env{h: r, d: deps}
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

func TestT19_AC3_HTTP_Authorization(t *testing.T) {
	e := newEnv(t, P, nil, nil)
	if c := e.do("GET", "/api/audit-log/usage", "").Code; c != 401 {
		t.Fatalf("anon %d", c)
	}
	if c := e.do("GET", "/api/audit-log/usage", "developer").Code; c != 403 {
		t.Fatalf("developer %d", c)
	}
	if c := e.do("GET", "/api/audit-log/usage", sa).Code; c != 200 {
		t.Fatalf("super_admin %d", c)
	}
}

func TestT19_AC3_HTTP_UnknownProject404(t *testing.T) {
	e := newEnv(t, P, nil, nil)
	w := e.do("GET", "/api/audit-log/usage?project_id="+uuid.NewString(), sa)
	if w.Code != 404 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestT19_HTTP_RouteNotShadowedByID(t *testing.T) {
	e := newEnv(t, P, nil, nil)
	if c := e.do("GET", "/api/audit-log/usage", sa).Code; c == http.StatusTeapot {
		t.Fatal("/usage jatuh ke /{id}")
	}
	for _, m := range []string{"POST", "PUT", "DELETE"} {
		if c := e.do(m, "/api/audit-log/usage", sa).Code; c != 405 {
			t.Fatalf("%s: %d", m, c)
		}
	}
}

func TestT19_HTTP_Validation(t *testing.T) {
	e := newEnv(t, P, nil, nil)
	for _, q := range []string{"range=1y", "project_id=xyz", "section=files"} {
		if c := e.do("GET", "/api/audit-log/usage?"+q, sa).Code; c != 400 {
			t.Fatalf("%s: %d", q, c)
		}
	}
}

func TestT19_HTTP_EndToEndShape(t *testing.T) {
	ts := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	e := newEnv(t, P, []listfiles.Item{
		{Path: "a.md", UpdatedAt: ts}, {Path: "b.md", UpdatedAt: ts}, {Path: "c.md", UpdatedAt: ts},
	}, nil)
	now := time.Now().UTC()
	read(t, e.d.DB, P, K1, "mcp.read_file", "a.md", "ok", now.Add(-time.Hour))
	read(t, e.d.DB, P, K1, "mcp.read_file", "deleted.md", "ok", now.Add(-time.Hour))
	w := e.do("GET", "/api/audit-log/usage?project_id="+P, sa)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	m := decode(t, w)
	top := m["top_files"].(map[string]any)
	items := top["items"].([]any)
	if top["available"] != true || len(items) != 2 || items[0].(map[string]any)["path"] == "" {
		t.Fatalf("top %v", top)
	}
	un := m["unread_files"].(map[string]any)
	ui := un["items"].([]any)
	if len(ui) != 2 || ui[0].(map[string]any)["path"] != "b.md" || ui[1].(map[string]any)["path"] != "c.md" || un["truncated"] != false {
		t.Fatalf("unread %v", un)
	}
	if ui[0].(map[string]any)["updated_at"] == "" {
		t.Fatal("updated_at kosong")
	}
	if m["project_id"] != P {
		t.Fatalf("project_id %v", m["project_id"])
	}
	if pk := m["per_key"].(map[string]any); pk["available"] != true || len(pk["items"].([]any)) != 1 {
		t.Fatalf("per_key %v", pk)
	}
}

func TestT19_HTTP_SectionAndFailSoft(t *testing.T) {
	e := newEnv(t, P, nil, errors.New("down"))
	m := decode(t, e.do("GET", "/api/audit-log/usage?project_id="+P, sa))
	un := m["unread_files"].(map[string]any)
	if un["available"] != false || un["error_code"] != "UNAVAILABLE" {
		t.Fatalf("unread %v", un)
	}
	m = decode(t, e.do("GET", "/api/audit-log/usage?section=keys", sa))
	if m["per_key"] == nil || m["top_files"] != nil || m["unread_files"] != nil || m["zero_queries"] != nil {
		t.Fatalf("%v", m)
	}
	m = decode(t, e.do("GET", "/api/audit-log/usage", sa))
	if tf := m["top_files"].(map[string]any); tf["error_code"] != "PROJECT_REQUIRED" {
		t.Fatalf("%v", tf)
	}
}

func TestT19_HTTP_NoContentLeakAndNoAudit(t *testing.T) {
	e := newEnv(t, P, nil, nil)
	now := time.Now().UTC()
	insert(t, e.d.DB, seed{ProjectID: P, ActorID: K1, Action: "mcp.search_knowledge",
		Args: `{"query":"x","secret":"s3cr3t"}`, Result: `{"total":0,"snippet":"ISI-RAHASIA"}`, At: now.Add(-time.Hour)})
	count := func() int {
		var n int
		if err := e.d.DB.Get(&n, `SELECT count(*) FROM audit_log`); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	w := e.do("GET", "/api/audit-log/usage?project_id="+P, sa)
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	body := w.Body.String()
	for _, s := range []string{"ISI-RAHASIA", "s3cr3t", "secret", "snippet"} {
		if strings.Contains(body, s) {
			t.Fatalf("bocor %q: %s", s, body)
		}
	}
	if !strings.Contains(body, `"query":"x"`) {
		t.Fatalf("query ternormalisasi hilang: %s", body)
	}
	if after := count(); after != before {
		t.Fatalf("audit bertambah %d -> %d", before, after)
	}
}
