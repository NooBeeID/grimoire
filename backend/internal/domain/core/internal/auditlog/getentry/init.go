// Package getentry adalah feature GET /api/audit-log/{id}.
package getentry

import (
	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
)

// Init memasang rute pada sub-router /audit-log (guard role dipasang pemanggil).
func Init(d app.Deps, r chi.Router) {
	h := NewHandler(NewUsecase(NewRepository(d.DB)))
	r.Get("/{id}", h.Get)
}
