package listentries

import (
	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
)

// Init merakit repository -> usecase -> handler dan mendaftarkan GET /.
// RequireRole(super_admin) dipasang di grup /audit-log oleh auditlog/base.go.
func Init(d app.Deps, r chi.Router) {
	h := NewHandler(NewUsecase(NewRepository(d.DB)))
	r.Get("/", h.List)
}
