package middleware

import (
	"net/http"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	useLogger(t)
	h := func(env map[string]string) http.Header {
		cfg := testConfig(t, env)
		return do(SecurityHeaders(cfg)(http.HandlerFunc(okHandler)), "GET", "/", nil, "").Header()
	}
	hd := h(nil)
	wantCSP := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	if hd.Get("X-Content-Type-Options") != "nosniff" || hd.Get("X-Frame-Options") != "DENY" ||
		hd.Get("Referrer-Policy") != "same-origin" || hd.Get("Content-Security-Policy") != wantCSP {
		t.Fatalf("%v", hd)
	}
	if hd.Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS tanpa COOKIE_SECURE")
	}
	hd = h(map[string]string{"COOKIE_SECURE": "true"})
	if hd.Get("Strict-Transport-Security") != "max-age=31536000; includeSubDomains" {
		t.Fatal("HSTS hilang")
	}
	hd = h(map[string]string{"APP_ENV": "development"})
	csp := hd.Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' ws:;") || strings.Contains(csp, "unsafe-eval") {
		t.Fatal(csp)
	}
}
