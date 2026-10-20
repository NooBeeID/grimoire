package getme

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan GET /auth/me (di bawah /api).
func Init(d app.Deps, _ base.Shared) {
	h := NewHandler(NewUsecase())
	d.API.With(middleware.RequireAuth()).Get("/auth/me", h.Get)
}
