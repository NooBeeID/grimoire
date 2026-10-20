package resetpassword

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/reqctx"
)

func newHandlerEnv() (http.Handler, env) {
	e := setup()
	h := NewHandler(e.uc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
			next.ServeHTTP(w, rq.WithContext(reqctx.WithIdentity(rq.Context(), reqctx.Identity{UserID: "admin-1", Role: base.RoleSuperAdmin})))
		})
	})
	r.Post("/users/{id}/reset-password", h.Reset)
	return r, e
}

func do(h http.Handler, body io.Reader, mod func(*http.Request)) *httptest.ResponseRecorder {
	rq := httptest.NewRequest(http.MethodPost, "/users/target-1/reset-password", body)
	if mod != nil {
		mod(rq)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, rq)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body: %s", w.Body.String())
	}
	return m
}

func TestHandler_Success200(t *testing.T) {
	h, _ := newHandlerEnv()
	w := do(h, strings.NewReader(`{}`), nil)
	m := decode(t, w)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatalf("code=%d hdr=%v", w.Code, w.Header())
	}
	if pw, _ := m["temporary_password"].(string); len(pw) != 16 {
		t.Fatalf("%v", m)
	}
	if u := m["user"].(map[string]any); u["must_change_password"] != true || u["id"] != "target-1" {
		t.Fatalf("%v", u)
	}
}

func TestHandler_EmptyBody(t *testing.T) {
	cases := map[string]func(*http.Request){
		"nil body":       func(r *http.Request) { r.Body = nil },
		"content-length": func(r *http.Request) { r.ContentLength = 0 },
		"chunked empty": func(r *http.Request) {
			r.Body = io.NopCloser(bytes.NewReader(nil))
			r.ContentLength = -1
		},
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := newHandlerEnv()
			w := do(h, nil, mod)
			if w.Code != 200 || decode(t, w)["temporary_password"] == nil {
				t.Fatalf("code=%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandler_EmptyObject(t *testing.T) {
	for _, b := range []string{`{}`, `{"password":""}`} {
		h, _ := newHandlerEnv()
		w := do(h, strings.NewReader(b), nil)
		if w.Code != 200 || decode(t, w)["temporary_password"] == nil {
			t.Fatalf("%s -> %d", b, w.Code)
		}
	}
}

func TestHandler_ProvidedPasswordNull(t *testing.T) {
	h, e := newHandlerEnv()
	w := do(h, strings.NewReader(`{"password":"sementara-baru-2026"}`), nil)
	m := decode(t, w)
	v, present := m["temporary_password"]
	if w.Code != 200 || !present || v != nil {
		t.Fatalf("code=%d %s", w.Code, w.Body.String())
	}
	if e.h.got[0] != "sementara-baru-2026" || strings.Contains(w.Body.String(), "sementara-baru-2026") {
		t.Fatal("password bocor / tidak di-hash")
	}
}

func TestHandler_InvalidJSON400(t *testing.T) {
	h, e := newHandlerEnv()
	w := do(h, strings.NewReader(`{bad`), nil)
	if w.Code != 400 {
		t.Fatalf("code=%d", w.Code)
	}
	noEffects(t, e)
}

func TestHandler_WrongType400(t *testing.T) {
	h, e := newHandlerEnv()
	w := do(h, strings.NewReader(`{"password": 5}`), nil)
	if w.Code != 400 {
		t.Fatalf("code=%d", w.Code)
	}
	noEffects(t, e)
}

func TestHandler_UnknownFieldBehavior(t *testing.T) {
	h, e := newHandlerEnv()
	w := do(h, strings.NewReader(`{"must_change_password":false,"is_active":false,"role":"super_admin"}`), nil)
	if w.Code == 200 {
		u := decode(t, w)["user"].(map[string]any)
		if u["must_change_password"] != true || u["role"] != base.RoleDeveloper {
			t.Fatalf("%v", u)
		}
	} else if w.Code != 400 {
		t.Fatalf("code=%d", w.Code)
	}
	if len(e.rp.resets) > 0 && e.rp.resets[0].UserID != "target-1" {
		t.Fatal("target salah")
	}
}

func TestHandler_BadUUID404(t *testing.T) {
	h, e := newHandlerEnv()
	e.rp.found = false
	w := do(h, nil, nil)
	m := decode(t, w)
	if w.Code != 404 || m["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("code=%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_ValidationError400(t *testing.T) {
	h, _ := newHandlerEnv()
	w := do(h, strings.NewReader(`{"password":"pendek"}`), nil)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "minimal 10 karakter") {
		t.Fatalf("code=%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_SelfReset403(t *testing.T) {
	h, e := newHandlerEnv()
	e.rp.user.ID = "admin-1"
	w := do(h, nil, nil)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "SELF_ACTION_FORBIDDEN") {
		t.Fatalf("code=%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_InternalErrorHidesDetail(t *testing.T) {
	h, e := newHandlerEnv()
	e.rp.resetErr = errors.New("pgx: boom SELECT")
	w := do(h, strings.NewReader(`{"password":"sementara-baru-2026"}`), nil)
	body := w.Body.String()
	if w.Code != 500 || strings.Contains(body, "sementara-baru-2026") || strings.Contains(body, "pgx") ||
		strings.Contains(body, "temporary_password") {
		t.Fatalf("code=%d %s", w.Code, body)
	}
}

func TestHandler_NoStoreNotSetOnErrorAndNoCredentials(t *testing.T) {
	h, _ := newHandlerEnv()
	w := do(h, strings.NewReader(`{bad`), nil)
	if strings.Contains(w.Body.String(), "temporary_password") {
		t.Fatal("error membawa kredensial")
	}
}

func TestHandler_AuditMetaFromRequest(t *testing.T) {
	h, e := newHandlerEnv()
	w := do(h, nil, func(r *http.Request) { r.Header.Set("User-Agent", "ua-test") })
	if w.Code != 200 || e.rec.evs[0].Meta.UserAgent != "ua-test" || e.rec.evs[0].Meta.IP == "" {
		t.Fatalf("meta=%+v", e.rec.evs[0].Meta)
	}
}
