package updateuser

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan PATCH /users/{id} (di bawah /api), khusus super_admin.
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(NewRepository(d.DB), audit.New(d.Bus, d.Log), s.Clock)
	h := NewHandler(uc)
	d.API.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin)).
		Patch("/users/{id}", h.Patch)
}
