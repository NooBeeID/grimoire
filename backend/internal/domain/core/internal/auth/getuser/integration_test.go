//go:build integration

package getuser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	Init(app.Deps{DB: d, API: api}, base.Shared{Clock: func() time.Time { return time.Now().UTC() }})
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

func insertInv(t *testing.T, s *store.Store, userID string, expires time.Time, accepted bool) {
	t.Helper()
	now := time.Now().UTC()
	err := s.InsertInvitation(context.Background(), base.Invitation{
		UserID: userID, InvitedBy: userID, IssuedAt: now.Add(-time.Hour), CreatedAt: now.Add(-time.Hour),
		ExpiresAt: expires, DeliveryStatus: base.DeliverySent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		if _, err := s.AcceptInvitation(context.Background(), userID, now); err != nil {
			t.Fatal(err)
		}
	}
}

func invOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	v, ok := m["invitation"]
	if !ok {
		t.Fatalf("kunci invitation hilang: %s", w.Body)
	}
	if v == nil {
		return nil
	}
	return v.(map[string]any)
}

func TestGetUser_Integration_InvitationMarker(t *testing.T) {
	h, s := router(t, base.RoleSuperAdmin)
	a := insertUser(t, s, true)
	insertInv(t, s, a.ID, time.Now().Add(24*time.Hour), false)
	w := get(h, "/api/users/"+a.ID)
	inv := invOf(t, w)
	if w.Code != 200 || inv == nil || inv["pending"] != true || inv["expired"] != false || inv["delivery_status"] != "sent" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	if _, bad := inv["issued_at"]; bad || strings.Contains(w.Body.String(), "argon2id") {
		t.Fatalf("bocor: %s", w.Body)
	}
}

func TestGetUser_Integration_InvitationExpired(t *testing.T) {
	h, s := router(t, base.RoleSuperAdmin)
	a := insertUser(t, s, true)
	insertInv(t, s, a.ID, time.Now().Add(-time.Hour), false)
	if inv := invOf(t, get(h, "/api/users/"+a.ID)); inv == nil || inv["expired"] != true {
		t.Fatalf("inv=%v", inv)
	}
}

func TestGetUser_Integration_InvitationNullWhenAcceptedOrAbsent(t *testing.T) {
	h, s := router(t, base.RoleSuperAdmin)
	b := insertUser(t, s, true)
	insertInv(t, s, b.ID, time.Now().Add(24*time.Hour), true)
	if inv := invOf(t, get(h, "/api/users/"+b.ID)); inv != nil {
		t.Fatalf("diterima harus null: %v", inv)
	}
	c, err := s.InsertUser(context.Background(), base.User{Email: "c@x.id", Name: "C", PasswordHash: "h", Role: base.RoleDeveloper, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if inv := invOf(t, get(h, "/api/users/"+c.ID)); inv != nil {
		t.Fatalf("tanpa undangan harus null: %v", inv)
	}
}
