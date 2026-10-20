// Package logout menangani POST /api/auth/logout: mencabut sesi saat ini.
package logout

import (
	"net/http"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/middleware"
)

// Params adalah input usecase Logout.
type Params struct {
	Token string           // nilai mentah cookie; "" bila cookie tidak ada
	Meta  base.RequestMeta // IP dan UserAgent untuk audit
}

// parseParams membaca cookie sesi dan metadata request. Tidak pernah gagal.
func parseParams(r *http.Request) Params {
	p := Params{Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())}}
	if c, err := r.Cookie(middleware.SessionCookieName); err == nil {
		p.Token = c.Value
	}
	return p
}
