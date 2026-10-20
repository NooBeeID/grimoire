package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"grimoire/lib/common/errs"
)

func TestCheckOrigin(t *testing.T) {
	allowed := []string{"https://App.Example.com"}
	cases := []struct {
		origin string
		want   bool
	}{
		{"", true}, {"http://example.com", true}, {"https://app.example.com", true},
		{"https://evil.example", false}, {"null", false}, {"%%%", false}, {"example.com", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://example.com/mcp", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := CheckOrigin(r, allowed); got != c.want {
			t.Errorf("%q: got %v", c.origin, got)
		}
	}
}

func TestOriginGuard_403(t *testing.T) {
	useLogger(t)
	h := OriginGuard(nil)(http.HandlerFunc(okHandler))
	w := do(h, "GET", "/", map[string]string{"Origin": "https://evil.example"}, "")
	e := decodeErr(t, w)
	if w.Code != 403 || e.Error.Code != "FORBIDDEN" || e.Error.Message != "origin tidak diizinkan" {
		t.Fatalf("%d %+v", w.Code, e)
	}
	if w := do(h, "GET", "/", nil, ""); w.Code != 200 {
		t.Fatal("tanpa origin harus lolos")
	}
}

var _ = context.Background
var _ = errs.ErrNotFound
