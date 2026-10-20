package middleware

import (
	"net/http"

	"grimoire/internal/infra/http/respond"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// PasswordChangeGate menolak (403 PASSWORD_CHANGE_REQUIRED) request user yang wajib
// mengganti password, kecuali endpoint di exempt (key "METHOD path"). Tanpa identitas
// atau tanpa flag, request lanjut.
func PasswordChangeGate(exempt map[string]struct{}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := reqctx.IdentityFrom(r.Context())
			if !ok || !id.MustChangePassword {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := exempt[r.Method+" "+r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}
			respond.WriteError(w, r, errs.New(errs.CodePasswordChangeRequired, "password wajib diganti sebelum melanjutkan"))
		})
	}
}
