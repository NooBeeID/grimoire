package middleware

import (
	"net/http"

	"grimoire/internal/infra/config"
)

// SecurityHeaders menaruh header keamanan sebelum handler berikutnya dipanggil.
func SecurityHeaders(cfg config.Config) func(http.Handler) http.Handler {
	connect := "'self'"
	if cfg.AppEnv == "development" {
		connect = "'self' ws:"
	}
	csp := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; font-src 'self'; connect-src " + connect +
		"; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("Content-Security-Policy", csp)
			if cfg.CookieSecure {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
