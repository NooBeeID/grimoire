package getme

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	httpx "grimoire/internal/infra/http"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

type env struct {
	root  http.Handler
	calls *int
}

// newEnv merakit router T00 lengkap + getme.Init + resolve-session palsu (tanpa DB).
func newEnv(t *testing.T, resolve func(call int, token string) (resolvesession.Response, error)) env {
	t.Helper()
	vars := map[string]string{
		"DATABASE_URL":   "postgres://u:p@localhost:5432/db",
		"SESSION_SECRET": strings.Repeat("s", 32),
		"COOKIE_SECURE":  "false",
	}
	cfg, err := config.Load(func(k string) string { return vars[k] })
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := bus.NewRegistry(log)
	n := new(int)
	var mu sync.Mutex
	reg.Register(constants.BUS_CORE_AUTH_RESOLVE_SESSION, func(_ context.Context, req any) (any, error) {
		r, err := bus.Expect[resolvesession.Request](req)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		*n++
		c := *n
		mu.Unlock()
		return resolve(c, r.Token)
	})
	root, api, mcp := httpx.NewRouter(cfg, log, reg, nil)
	Init(app.Deps{Config: cfg, Log: log, Bus: reg, API: api, MCP: mcp}, base.Shared{})
	api.Get("/_probe", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	return env{root: root, calls: n}
}

func (e env) do(method, path string, cookie bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = "10.0.0.5:4321"
	r.Header.Set("X-Requested-With", "grimoire")
	if cookie {
		r.AddCookie(&http.Cookie{Name: "grimoire_session", Value: "tok"})
	}
	w := httptest.NewRecorder()
	e.root.ServeHTTP(w, r)
	return w
}

func okSession(role string, must bool) func(int, string) (resolvesession.Response, error) {
	return func(int, string) (resolvesession.Response, error) {
		return resolvesession.Response{Found: true, UserID: "u1", Email: "a@b.id", Name: "A", Role: role, MustChangePassword: must}, nil
	}
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("body=%q", w.Body.String())
	}
	if b.TraceID == "" {
		t.Fatal("tanpa trace_id")
	}
	return b.Error.Code
}

func TestT01_AC4_GetMe_ExemptFromPasswordGate(t *testing.T) {
	e := newEnv(t, okSession("developer", true))
	w := e.do(http.MethodGet, "/api/auth/me", true)
	if w.Code != 200 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	var me base.MeDTO
	if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil || !me.MustChangePassword || me.ID != "u1" {
		t.Fatalf("%+v %v", me, err)
	}
	p := e.do(http.MethodGet, "/api/_probe", true)
	if p.Code != 403 || errCode(t, p) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("probe code=%d body=%s", p.Code, p.Body)
	}
}

func TestGetMe_Unauthenticated_401(t *testing.T) {
	e := newEnv(t, okSession("developer", false))
	w := e.do(http.MethodGet, "/api/auth/me", false)
	if w.Code != 401 || errCode(t, w) != "UNAUTHORIZED" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	if *e.calls != 0 {
		t.Fatal("bus dipanggil tanpa cookie")
	}
}

func TestGetMe_InvalidSession_401(t *testing.T) {
	e := newEnv(t, func(int, string) (resolvesession.Response, error) { return resolvesession.Response{}, nil })
	w := e.do(http.MethodGet, "/api/auth/me", true)
	if w.Code != 401 || errCode(t, w) != "UNAUTHORIZED" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("cookie=%q", w.Header().Get("Set-Cookie"))
	}
}

func TestGetMe_MethodNotAllowed(t *testing.T) {
	e := newEnv(t, okSession("developer", false))
	w := e.do(http.MethodPost, "/api/auth/me", true)
	if w.Code != 405 || errCode(t, w) != "METHOD_NOT_ALLOWED" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestGetMe_NoExtraQueries(t *testing.T) {
	e := newEnv(t, okSession("developer", false))
	if w := e.do(http.MethodGet, "/api/auth/me", true); w.Code != 200 {
		t.Fatalf("code=%d", w.Code)
	}
	if *e.calls != 1 {
		t.Fatalf("resolve-session dipanggil %d kali", *e.calls)
	}
}

func TestGetMe_ReflectsRoleChangeNextRequest(t *testing.T) {
	e := newEnv(t, func(call int, _ string) (resolvesession.Response, error) {
		role := "developer"
		if call > 1 {
			role = "super_admin"
		}
		return okSession(role, false)(call, "")
	})
	roles := []string{}
	for i := 0; i < 2; i++ {
		w := e.do(http.MethodGet, "/api/auth/me", true)
		var me base.MeDTO
		if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil {
			t.Fatal(err)
		}
		roles = append(roles, me.Role)
	}
	if roles[0] != "developer" || roles[1] != "super_admin" {
		t.Fatalf("%v", roles)
	}
}
