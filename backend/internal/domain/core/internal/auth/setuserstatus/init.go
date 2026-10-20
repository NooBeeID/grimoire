package setuserstatus

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan POST /users/{id}/deactivate dan /activate (di bawah /api), khusus super_admin.
func Init(d app.Deps, s base.Shared) {
	h := NewHandler(NewUsecase(NewRepo(d.DB), audit.New(d.Bus, d.Log), s.Clock))
	admin := d.API.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin))
	admin.Post("/users/{id}/deactivate", h.Deactivate)
	admin.Post("/users/{id}/activate", h.Activate)
}
