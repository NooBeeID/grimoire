package authenticate

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExtractKey(t *testing.T) {
	cases := []struct {
		name string
		h    map[string]string
		want string
	}{
		{"bearer", map[string]string{"Authorization": "Bearer abc"}, "abc"},
		{"bearer lower", map[string]string{"Authorization": "bearer abc"}, "abc"},
		{"bearer upper", map[string]string{"Authorization": "BEARER abc"}, "abc"},
		{"x-api-key trimmed", map[string]string{"X-API-Key": "  k1 "}, "k1"},
		{"bearer wins", map[string]string{"Authorization": "Bearer a", "X-API-Key": "b"}, "a"},
		{"basic only", map[string]string{"Authorization": "Basic xxx"}, ""},
		{"basic plus xkey", map[string]string{"Authorization": "Basic xxx", "X-API-Key": "k"}, "k"},
		{"empty bearer plus xkey", map[string]string{"Authorization": "Bearer ", "X-API-Key": "k"}, "k"},
		{"none", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range c.h {
				h.Set(k, v)
			}
			if got := extractKey(h); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestParseInput(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.RemoteAddr = "10.0.0.7:5555"
	r.Header.Set("User-Agent", strings.Repeat("é", 300))
	in := ParseInput(r)
	if in.IP != "10.0.0.7" {
		t.Fatalf("ip %q", in.IP)
	}
	if len(in.UserAgent) > 256 || !utf8.ValidString(in.UserAgent) || in.UserAgent == "" {
		t.Fatalf("ua len=%d", len(in.UserAgent))
	}
}
