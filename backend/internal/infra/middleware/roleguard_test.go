package middleware

import (
	"net/http"
	"testing"

	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

func withIdentity(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if role != "" {
				r = r.WithContext(reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: "u", Role: role}))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TestRequireAuth(t *testing.T) {
	useLogger(t)
	if w := do(RequireAuth()(http.HandlerFunc(okHandler)), "GET", "/", nil, ""); w.Code != 401 || decodeErr(t, w).Error.Code != "UNAUTHORIZED" {
		t.Fatal(w.Code)
	}
	if w := do(withIdentity("developer")(RequireAuth()(http.HandlerFunc(okHandler))), "GET", "/", nil, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestRequireRole(t *testing.T) {
	useLogger(t)
	g := RequireRole(resolvesession.RoleSuperAdmin)(http.HandlerFunc(okHandler))
	if w := do(g, "GET", "/", nil, ""); w.Code != 401 {
		t.Fatalf("anonim: %d", w.Code)
	}
	w := do(withIdentity("developer")(g), "GET", "/", nil, "")
	if w.Code != 403 || decodeErr(t, w).Error.Code != "FORBIDDEN" {
		t.Fatalf("developer: %d", w.Code)
	}
	if w := do(withIdentity("super_admin")(g), "GET", "/", nil, ""); w.Code != 200 {
		t.Fatalf("admin: %d", w.Code)
	}
	multi := RequireRole("developer", "super_admin")(http.HandlerFunc(okHandler))
	if w := do(withIdentity("developer")(multi), "GET", "/", nil, ""); w.Code != 200 {
		t.Fatal("multi role")
	}
	mustPanic(t, func() { RequireRole() })
}
