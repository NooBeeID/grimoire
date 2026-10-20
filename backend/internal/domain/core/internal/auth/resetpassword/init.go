package resetpassword

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan POST /users/{id}/reset-password (di bawah /api), khusus super_admin.
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(NewRepo(d.DB), s.Hasher, audit.New(d.Bus, d.Log), s.Clock, s.Rand, s.Password)
	h := NewHandler(uc)
	d.API.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin)).
		Post("/users/{id}/reset-password", h.Reset)
}
