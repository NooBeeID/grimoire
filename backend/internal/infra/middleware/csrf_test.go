package middleware

import (
	"net/http"
	"testing"
)

func TestCSRF(t *testing.T) {
	useLogger(t)
	h := CSRF()(http.HandlerFunc(okHandler))
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		w := do(h, m, "/", nil, "")
		e := decodeErr(t, w)
		if w.Code != 403 || e.Error.Code != "CSRF_REQUIRED" || e.Error.Message != "header X-Requested-With wajib untuk request mutasi" {
			t.Fatalf("%s: %d %+v", m, w.Code, e)
		}
		for _, bad := range []string{"grimoire2", "Grimoire", "wrong"} {
			if w := do(h, m, "/", map[string]string{CSRFHeader: bad}, ""); w.Code != 403 {
				t.Fatalf("%s %q lolos", m, bad)
			}
		}
		if w := do(h, m, "/", map[string]string{CSRFHeader: CSRFValue}, ""); w.Code != 200 {
			t.Fatalf("%s dengan header benar: %d", m, w.Code)
		}
	}
	for _, m := range []string{"GET", "HEAD", "OPTIONS"} {
		if w := do(h, m, "/", nil, ""); w.Code != 200 {
			t.Fatalf("%s: %d", m, w.Code)
		}
	}
}
