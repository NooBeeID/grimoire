package authenticate

import (
	"net/http"
	"strings"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/infra/middleware"
)

// Input adalah hasil parsing request admisi. RawKey tidak pernah di-log.
type Input struct {
	RawKey    string // kosong = tidak ada key
	IP        string
	UserAgent string
}

// ParseInput mengekstrak key, IP (RemoteAddr), dan User-Agent.
func ParseInput(r *http.Request) Input {
	return Input{
		RawKey:    extractKey(r.Header),
		IP:        middleware.KeyByIP(r),
		UserAgent: base.TruncateName(r.UserAgent(), 256),
	}
}

// extractKey: Bearer menang atas X-API-Key; skema lain diabaikan.
func extractKey(h http.Header) string {
	if scheme, tok, ok := strings.Cut(h.Get("Authorization"), " "); ok && strings.EqualFold(scheme, "Bearer") {
		if tok = strings.TrimSpace(tok); tok != "" {
			return tok
		}
	}
	return strings.TrimSpace(h.Get("X-API-Key"))
}
