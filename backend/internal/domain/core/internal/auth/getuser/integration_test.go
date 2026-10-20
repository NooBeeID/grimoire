//go:build integration

package getuser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/migrations"
)

// router memasang Init pada chi nyata; identitas disuntik (role kosong = tanpa sesi).
func router(t *testing.T, role string) (http.Handler, *store.Store) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	api := chi.NewRouter()
	if role != "" {
		api.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(reqctx.WithIdentity(r.Context(),
					reqctx.Identity{UserID: "u", Role: role})))
			})
		})
	}
	Init(app.Deps{DB: d, API: api}, base.Shared{})
	root := chi.NewRouter()
	root.Mount("/api", api)
	return root, store.New(d)
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func insertUser(t *testing.T, s *store.Store, active bool) base.User {
	t.Helper()
	u, err := s.InsertUser(context.Background(), base.User{Email: "a@x.id", Name: "N",
		PasswordHash: "$argon2id$secret", Role: base.RoleDeveloper, IsActive: active})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestGetUser_Integration_Found(t *testing.T) {
	h, s := router(t, base.RoleSuperAdmin)
	u := insertUser(t, s, true)
	w := get(h, "/api/users/"+u.ID)
	if w.Code != 200 || !strings.Contains(w.Body.String(), u.ID) || !strings.Contains(w.Body.String(), "a@x.id") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "argon2id") || strings.Contains(w.Body.String(), "password_hash") {
		t.Fatalf("bocor hash: %s", w.Body)
	}
}

func TestGetUser_Integration_InactiveUser_200(t *testing.T) {
	h, s := router(t, base.RoleSuperAdmin)
	u := insertUser(t, s, false)
	w := get(h, "/api/users/"+u.ID)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"is_active":false`) {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestGetUser_Integration_NotFound_404(t *testing.T) {
	h, _ := router(t, base.RoleSuperAdmin)
	for _, id := range []string{"7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02", "abc", "123", "%27%3B--"} {
		w := get(h, "/api/users/"+id)
		if w.Code != 404 || !strings.Contains(w.Body.String(), "NOT_FOUND") {
			t.Fatalf("id=%s code=%d body=%s", id, w.Code, w.Body)
		}
	}
}

func TestT01_AC7_GetUser_DeveloperForbidden(t *testing.T) {
	h, s := router(t, base.RoleDeveloper)
	u := insertUser(t, s, true)
	w := get(h, "/api/users/"+u.ID)
	if w.Code != 403 || strings.Contains(w.Body.String(), u.Email) {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestGetUser_Unauthenticated_401(t *testing.T) {
	h, _ := router(t, "")
	if w := get(h, "/api/users/x"); w.Code != 401 {
		t.Fatalf("code=%d", w.Code)
	}
}
