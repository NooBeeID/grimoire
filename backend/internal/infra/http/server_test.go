package httpx

import (
	"github.com/go-chi/chi/v5"
	"grimoire/internal/infra/middleware"
	"net/http"
	"strings"
	"testing"
	"time"

	"grimoire/lib/contracts/core/auth/resolvesession"
)

func TestNotFoundJSON(t *testing.T) {
	s := newStack(t, nil, nil)
	for _, p := range []string{"/api/nope", "/mcp/nope", "/nope"} {
		w := do(s.root, "GET", p, nil, "")
		e := decodeErr(t, w)
		if w.Code != 404 || e.Error.Code != "NOT_FOUND" || e.TraceID == "" || e.TraceID != w.Header().Get(TraceHeader) {
			t.Fatalf("%s: %d %+v", p, w.Code, e)
		}
	}
}

func TestMethodNotAllowed_AllowHeader(t *testing.T) {
	s := newStack(t, nil, nil)
	w := do(s.root, "DELETE", "/healthz", nil, "")
	e := decodeErr(t, w)
	if w.Code != 405 || e.Error.Code != "METHOD_NOT_ALLOWED" || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("%d %+v allow=%q", w.Code, e, w.Header().Get("Allow"))
	}
}

func TestAPI_MethodNotAllowedJSON(t *testing.T) {
	s := newStack(t, nil, nil)
	s.api.Get("/things", okHandler)
	w := do(s.root, "PUT", "/api/things", map[string]string{middleware.CSRFHeader: middleware.CSRFValue}, "")
	if w.Code != 405 || w.Header().Get("Allow") != "GET" || decodeErr(t, w).Error.Code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("%d allow=%q", w.Code, w.Header().Get("Allow"))
	}
	if do(s.root, "GET", "/api/things", nil, "").Code != 200 {
		t.Fatal("route harus jalan")
	}
}

func TestAPI_MutationOn404StillCSRFChecked(t *testing.T) {
	s := newStack(t, nil, nil)
	w := do(s.root, "POST", "/api/tidak-ada", nil, "")
	if w.Code != 403 || decodeErr(t, w).Error.Code != "CSRF_REQUIRED" {
		t.Fatalf("%d", w.Code)
	}
	if w := do(s.root, "POST", "/api/tidak-ada", map[string]string{middleware.CSRFHeader: middleware.CSRFValue}, ""); w.Code != 404 {
		t.Fatalf("dengan header: %d", w.Code)
	}
}

func TestT00_AC6_CSRFApiAndMCPExempt(t *testing.T) {
	s := newStack(t, nil, nil)
	for _, bad := range []string{"", "wrong"} {
		hdr := map[string]string{}
		if bad != "" {
			hdr[middleware.CSRFHeader] = bad
		}
		w := do(s.root, "POST", "/api/anything", hdr, "")
		if w.Code != 403 || decodeErr(t, w).Error.Code != "CSRF_REQUIRED" {
			t.Fatalf("%q: %d", bad, w.Code)
		}
	}
	if w := do(s.root, "POST", "/mcp/anything", nil, ""); w.Code != 404 {
		t.Fatalf("mcp harus bebas CSRF (404): %d", w.Code)
	}
}

func TestAPI_NoCookieNoBusCall(t *testing.T) {
	s := newStack(t, nil, nil)
	n := registerSession(s.bus, func(string) (resolvesession.Response, error) { return resolvesession.Response{}, nil })
	if w := do(s.root, "GET", "/api/x", nil, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if *n != 0 {
		t.Fatal("bus tidak boleh dipanggil")
	}
}

func TestAPI_SessionAndCSRFOrder(t *testing.T) {
	s := newStack(t, nil, nil)
	n := registerSession(s.bus, func(string) (resolvesession.Response, error) { return resolvesession.Response{}, nil })
	w := do(s.root, "POST", "/api/x", map[string]string{"Cookie": middleware.SessionCookieName + "=abc"}, "")
	if w.Code != 403 || *n != 0 {
		t.Fatalf("%d bus=%d", w.Code, *n)
	}
}

func TestAPI_NoProviderRegistered500(t *testing.T) {
	s := newStack(t, nil, nil)
	w := do(s.root, "GET", "/api/x", map[string]string{"Cookie": middleware.SessionCookieName + "=abc"}, "")
	e := decodeErr(t, w)
	if w.Code != 500 || e.Error.Message != "terjadi kesalahan internal" || strings.Contains(w.Body.String(), "resolve-session") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("cookie tidak boleh dihapus")
	}
}

func TestAPI_AuthenticatedRoleRoutes(t *testing.T) {
	s := newStack(t, nil, nil)
	registerSession(s.bus, func(tok string) (resolvesession.Response, error) {
		return resolvesession.Response{Found: true, UserID: "u-" + tok, Role: tok}, nil
	})
	s.api.With(middleware.RequireAuth()).Get("/me", okHandler)
	s.api.With(middleware.RequireRole("super_admin")).Get("/admin", okHandler)
	ck := func(role string) map[string]string {
		return map[string]string{"Cookie": middleware.SessionCookieName + "=" + role}
	}
	if do(s.root, "GET", "/api/me", nil, "").Code != 401 {
		t.Fatal("anonim harus 401")
	}
	if do(s.root, "GET", "/api/me", ck("developer"), "").Code != 200 {
		t.Fatal("me")
	}
	if do(s.root, "GET", "/api/admin", ck("developer"), "").Code != 403 {
		t.Fatal("developer ke admin harus 403")
	}
	if do(s.root, "GET", "/api/admin", ck("super_admin"), "").Code != 200 {
		t.Fatal("admin")
	}
}

func TestAPI_DefaultTimeoutApplied(t *testing.T) {
	s := newStack(t, map[string]string{"HTTP_DEFAULT_TIMEOUT": "7s"}, nil)
	var rem time.Duration
	s.api.Get("/d", func(w http.ResponseWriter, r *http.Request) {
		dl, ok := r.Context().Deadline()
		if ok {
			rem = time.Until(dl)
		}
		okHandler(w, r)
	})
	do(s.root, "GET", "/api/d", nil, "")
	if rem < 5*time.Second || rem > 7*time.Second {
		t.Fatalf("sisa %v", rem)
	}
}

func TestAPI_BodyLimitDefaultAndOverride(t *testing.T) {
	s := newStack(t, map[string]string{"HTTP_DEFAULT_BODY_BYTES": "64"}, nil)
	s.api.Post("/in", readBody)
	s.api.With(middleware.BodyLimit(1024)).Post("/big", readBody)
	hdr := map[string]string{middleware.CSRFHeader: middleware.CSRFValue}
	body := `{"a":"` + strings.Repeat("x", 200) + `"}`
	w := do(s.root, "POST", "/api/in", hdr, body)
	e := decodeErr(t, w)
	d, _ := e.Error.Details.(map[string]any)
	if w.Code != 413 || d["limit_bytes"] != float64(64) {
		t.Fatalf("%d %+v", w.Code, e)
	}
	if w := do(s.root, "POST", "/api/big", hdr, body); w.Code != 200 {
		t.Fatalf("override naik: %d", w.Code)
	}
}

func TestMiddlewareOrder_PanicFromRoute(t *testing.T) {
	s := newStack(t, map[string]string{"COOKIE_SECURE": "true"}, nil)
	s.api.Get("/boom", func(w http.ResponseWriter, r *http.Request) { panic("kaboom") })
	w := do(s.root, "GET", "/api/boom", nil, "")
	e := decodeErr(t, w)
	tid := w.Header().Get(TraceHeader)
	if w.Code != 500 || e.Error.Message != "terjadi kesalahan internal" || tid == "" || e.TraceID != tid {
		t.Fatalf("%d %+v", w.Code, e)
	}
	if w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("security headers hilang pada 500")
	}
	m := s.logs.find(t, "http request")
	if m == nil || m["status"] != float64(500) || m["trace_id"] != tid || m["route"] != "/api/boom" {
		t.Fatalf("%v", m)
	}
	if strings.Contains(w.Body.String(), "kaboom") {
		t.Fatal("bocor")
	}
	if do(s.root, "GET", "/healthz", nil, "").Code != 200 {
		t.Fatal("proses harus tetap hidup")
	}
}

func TestMiddlewareOrder_Observed(t *testing.T) {
	log, _ := useLogger(t)
	var order []string
	mark := func(n string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, n)
				next.ServeHTTP(w, r)
			})
		}
	}
	r := chi.NewRouter()
	r.Use(middleware.Recover(log), mark("after-recover"), middleware.TraceID(), mark("inner-panic"))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) { panic("x") })
	w := do(r, "GET", "/", nil, "")
	if w.Code != 500 || strings.Join(order, ",") != "after-recover,inner-panic" {
		t.Fatalf("%d %v", w.Code, order)
	}
}

func TestNewServer_Settings(t *testing.T) {
	log, _ := useLogger(t)
	cfg := testConfig(t, map[string]string{"HTTP_ADDR": ":9999", "HTTP_READ_HEADER_TIMEOUT": "3s"})
	srv := NewServer(cfg, log, http.NewServeMux())
	if srv.Addr != ":9999" || srv.ReadHeaderTimeout != 3*time.Second || srv.IdleTimeout != 120*time.Second || srv.WriteTimeout != 0 {
		t.Fatalf("%+v", srv.Server)
	}
}
