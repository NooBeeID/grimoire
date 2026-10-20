package setuserstatus

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/reqctx"
)

func newRouter(tx *fakeTx) (http.Handler, *fakeRec) {
	uc, rec := setup(tx)
	h := NewHandler(uc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
			next.ServeHTTP(w, rq.WithContext(reqctx.WithIdentity(rq.Context(), reqctx.Identity{UserID: "admin-1", Email: "a@x.id", Role: base.RoleSuperAdmin})))
		})
	})
	r.Post("/users/{id}/deactivate", h.Deactivate)
	r.Post("/users/{id}/activate", h.Activate)
	return r, rec
}

func call(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rq := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, rq)
	return w
}

func TestHandler_RoutesAndStatus(t *testing.T) {
	h, rec := newRouter(&fakeTx{adminIDs: []string{"a", "b"}, user: devUser(true), found: true})
	w := call(h, "/users/dev-1/deactivate", "")
	var dto base.UserDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil || w.Code != 200 || dto.IsActive {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" || len(rec.evs) != 1 || rec.evs[0].Action != "user.deactivated" {
		t.Fatalf("%v %+v", w.Header(), rec.evs)
	}

	h, rec = newRouter(&fakeTx{user: devUser(false), found: true})
	w = call(h, "/users/dev-1/activate", "")
	if w.Code != 200 || len(rec.evs) != 1 || rec.evs[0].Action != "user.activated" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	self := base.User{ID: "admin-1", Role: base.RoleSuperAdmin, IsActive: true}
	last := base.User{ID: "admin-2", Role: base.RoleSuperAdmin, IsActive: true}
	cases := []struct {
		name string
		tx   *fakeTx
		want int
	}{
		{"self", &fakeTx{adminIDs: []string{"admin-1", "x"}, user: self, found: true}, 403},
		{"last", &fakeTx{adminIDs: []string{"admin-2"}, user: last, found: true}, 409},
		{"notfound", &fakeTx{}, 404},
	}
	for _, c := range cases {
		h, _ := newRouter(c.tx)
		if w := call(h, "/users/x/deactivate", ""); w.Code != c.want {
			t.Fatalf("%s: %d %s", c.name, w.Code, w.Body)
		}
	}
}

func TestHandler_IgnoresBody(t *testing.T) {
	h, _ := newRouter(&fakeTx{user: devUser(false), found: true})
	if w := call(h, "/users/dev-1/activate", "{{{ sampah"); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
