package updateuser

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
)

// Params adalah masukan usecase UpdateUser.
type Params struct {
	ID    string // dari URL; divalidasi uuid oleh usecase
	Name  *string
	Email *string
	Role  *string
	Meta  base.RequestMeta
}

type body struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
	Role  *string `json:"role"`
}

// parseParams membaca body JSON dan URL. Minimal satu field harus terisi.
func parseParams(r *http.Request) (Params, error) {
	var b body
	if err := httpx.DecodeJSON(r, &b); err != nil {
		return Params{}, err
	}
	if b.Name == nil && b.Email == nil && b.Role == nil {
		return Params{}, base.InvalidFields(base.FieldErrors{"_": "minimal satu field harus diisi"})
	}
	return Params{
		ID: chi.URLParam(r, "id"), Name: b.Name, Email: b.Email, Role: b.Role,
		Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())},
	}, nil
}
