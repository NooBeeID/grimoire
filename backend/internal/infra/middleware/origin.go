package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"
	"net/url"
	"strings"

	"grimoire/lib/common/errs"
)

// CheckOrigin: Origin kosong (klien non-browser) diizinkan; "null" ditolak; selain itu
// harus sama dengan Host request atau cocok dengan salah satu entri allowed.
func CheckOrigin(r *http.Request, allowed []string) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	if o == "null" {
		return false
	}
	u, err := url.Parse(o)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, a := range allowed {
		if strings.EqualFold(a, o) {
			return true
		}
	}
	return false
}

// OriginGuard menolak request dengan Origin yang tidak diizinkan (403).
func OriginGuard(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !CheckOrigin(r, allowed) {
				respond.WriteError(w, r, errs.New(errs.CodeForbidden, "origin tidak diizinkan"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
