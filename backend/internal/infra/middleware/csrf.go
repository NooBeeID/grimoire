package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"

	"grimoire/lib/common/errs"
)

const (
	CSRFHeader = "X-Requested-With"
	CSRFValue  = "grimoire"
)

// CSRF mewajibkan header X-Requested-With: grimoire pada method mutasi.
func CSRF() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				if r.Header.Get(CSRFHeader) != CSRFValue {
					respond.WriteError(w, r, errs.New(errs.CodeCSRFRequired, "header X-Requested-With wajib untuk request mutasi"))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
