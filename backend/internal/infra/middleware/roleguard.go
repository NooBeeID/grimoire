package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"
	"slices"

	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// RequireAuth menolak request tanpa identitas sesi (401).
func RequireAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := reqctx.IdentityFrom(r.Context()); !ok {
				respond.WriteError(w, r, errs.New(errs.CodeUnauthorized, "perlu login"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole mengizinkan hanya role tertentu. Tanpa identitas 401, role lain 403.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	if len(roles) == 0 {
		panic("httpx: RequireRole membutuhkan minimal satu role")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := reqctx.IdentityFrom(r.Context())
			if !ok {
				respond.WriteError(w, r, errs.New(errs.CodeUnauthorized, "perlu login"))
				return
			}
			if !slices.Contains(roles, id.Role) {
				respond.WriteError(w, r, errs.New(errs.CodeForbidden, "akses ditolak"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
