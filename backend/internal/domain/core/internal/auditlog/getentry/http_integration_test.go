//go:build integration

package getentry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

// withIdentity memasang identitas uji berdasar header X-Test-Role (kosong = anonim).
func withIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role := r.Header.Get("X-Test-Role"); role != "" {
			r = r.WithContext(reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: uuid.NewString(), Role: role}))
		}
		next.ServeHTTP(w, r)
	})
}

func integrationRouter(d app.Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(withIdentity)
	r.Route("/api/audit-log", func(r chi.Router) {
		r.Use(middleware.RequireRole(resolvesession.RoleSuperAdmin))
		Init(d, r)
	})
	return r
}

func call(h http.Handler, method, path, role string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if role != "" {
		req.Header.Set("X-Test-Role", role)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func seedEntry(t *testing.T, d app.Deps) string {
	t.Helper()
	id := uuid.NewString()
	insert(t, d.DB, id, insertFull, id, time.Now(), uuid.NewString(), uuid.NewString(),
		`{"path":"a.md","api_key":"[REDACTED]"}`, `{"bytes":12,"content":"[OMITTED]"}`, "203.0.113.5")
	return id
}

func TestT02_AC5_GetEntry_Roles(t *testing.T) {
	d := app.Deps{DB: testDB(t)}
	h := integrationRouter(d)
	id := seedEntry(t, d)
	path := "/api/audit-log/" + id

	if w := call(h, http.MethodGet, path, ""); w.Code != 401 || errCode(t, w) != "UNAUTHORIZED" {
		t.Fatalf("anonim: %d %s", w.Code, w.Body.String())
	}
	if w := call(h, http.MethodGet, path, "developer"); w.Code != 403 || errCode(t, w) != "FORBIDDEN" {
		t.Fatalf("developer: %d %s", w.Code, w.Body.String())
	}
	if w := call(h, http.MethodGet, "/api/audit-log/"+uuid.NewString(), "developer"); w.Code != 403 {
		t.Fatalf("developer id tak ada harus 403: %d", w.Code)
	}
	w := call(h, http.MethodGet, path, resolvesession.RoleSuperAdmin)
	if w.Code != 200 {
		t.Fatalf("super_admin: %d %s", w.Code, w.Body.String())
	}
	m := decode(t, w)
	if m["id"] != id || m["ip"] != "203.0.113.5" || m["args"].(map[string]any)["api_key"] != "[REDACTED]" {
		t.Fatalf("detail: %v", m)
	}
	if w := call(h, http.MethodGet, "/api/audit-log/"+uuid.NewString(), resolvesession.RoleSuperAdmin); w.Code != 404 || errCode(t, w) != "NOT_FOUND" {
		t.Fatalf("tak ada: %d %s", w.Code, w.Body.String())
	}
	if w := call(h, http.MethodGet, "/api/audit-log/not-a-uuid", resolvesession.RoleSuperAdmin); w.Code != 404 {
		t.Fatalf("invalid: %d", w.Code)
	}
}

func TestT02_AC7_NoMutationRoutes(t *testing.T) {
	d := app.Deps{DB: testDB(t)}
	h := integrationRouter(d)
	id := seedEntry(t, d)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if w := call(h, m, "/api/audit-log/"+id, resolvesession.RoleSuperAdmin); w.Code != 405 {
			t.Errorf("%s: %d %s", m, w.Code, w.Body.String())
		}
	}
	var n int
	if err := d.DB.GetContext(context.Background(), &n, `SELECT count(*) FROM audit_log WHERE id = $1 AND action = 'mcp.read_file'`, id); err != nil || n != 1 {
		t.Fatalf("baris berubah: n=%d err=%v", n, err)
	}
}

func TestT02_AC3_DetailAfterReferencesDeleted(t *testing.T) {
	d := app.Deps{DB: testDB(t)}
	h := integrationRouter(d)
	id := seedEntry(t, d)
	w := call(h, http.MethodGet, "/api/audit-log/"+id, resolvesession.RoleSuperAdmin)
	m := decode(t, w)
	if w.Code != 200 || m["project_label"] != "Payments" || m["actor_label"] != "claude-code-budi (gk_ab12)" {
		t.Fatalf("%d %v", w.Code, m)
	}
}
