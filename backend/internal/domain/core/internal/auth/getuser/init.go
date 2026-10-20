package getuser

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan GET /users/{id} (di bawah /api), khusus super_admin.
func Init(d app.Deps, _ base.Shared) {
	h := NewHandler(NewUsecase(NewRepository(d.DB)))
	d.API.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin)).
		Get("/users/{id}", h.Get)
}
