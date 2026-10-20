package updateuser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

func newHandlerEnv(e env) http.Handler {
	h := NewHandler(e.uc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
			next.ServeHTTP(w, rq.WithContext(reqctx.WithIdentity(rq.Context(), reqctx.Identity{UserID: actorID, Role: base.RoleSuperAdmin})))
		})
	})
	r.Patch("/users/{id}", h.Patch)
	return r
}

func do(h http.Handler, id, body string) *httptest.ResponseRecorder {
	rq := httptest.NewRequest(http.MethodPatch, "/users/"+id, strings.NewReader(body))
	rq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, rq)
	return w
}

func TestParseParams_EmptyAndNull(t *testing.T) {
	for _, b := range []string{`{}`, `{"name":null}`} {
		rq := httptest.NewRequest(http.MethodPatch, "/users/x", strings.NewReader(b))
		_, err := parseParams(rq)
		ee, ok := err.(*errs.Error)
		if !ok || ee.Code != errs.CodeInvalid {
			t.Fatalf("%s: %v", b, err)
		}
		fe, _ := detailsOf(ee)["fields"].(base.FieldErrors)
		if fe["_"] == "" {
			t.Fatalf("%s: %v", b, detailsOf(ee))
		}
	}
}

func TestParseParams_Rejected(t *testing.T) {
	for _, b := range []string{`{"is_active":false}`, `{"password":"x"}`, `{bukan json`, ``} {
		h := newHandlerEnv(setup())
		if w := do(h, targetID, b); w.Code != 400 {
			t.Errorf("%q: %d %s", b, w.Code, w.Body)
		}
	}
}

func TestHandler_Patch_Returns200WithUserDTO(t *testing.T) {
	e := setup()
	e.rp.user.PasswordHash = "$argon2id$rahasia"
	w := do(newHandlerEnv(e), targetID, `{"name":"Baru"}`)
	if w.Code != 200 || strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "argon2") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if m["name"] != "Baru" || m["id"] != targetID {
		t.Fatalf("%v", m)
	}
}

func TestHandler_Patch_ErrorStatusMapping(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*env)
		id    string
		body  string
		code  int
	}{
		{"self demote", func(e *env) { e.rp.user.ID, e.rp.user.Role = actorID, base.RoleSuperAdmin }, actorID, `{"role":"developer"}`, 403},
		{"last admin", func(e *env) { e.rp.user.Role = base.RoleSuperAdmin; e.rp.ids = []string{targetID} }, targetID, `{"role":"developer"}`, 409},
		{"email taken", func(e *env) { e.rp.updateErr = base.ErrEmailTaken }, targetID, `{"email":"a@b.id"}`, 409},
		{"bad id", func(*env) {}, "bukan-uuid", `{"name":"x"}`, 404},
		{"invalid field", func(*env) {}, targetID, `{"name":" "}`, 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup()
			c.setup(&e)
			if w := do(newHandlerEnv(e), c.id, c.body); w.Code != c.code {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}
