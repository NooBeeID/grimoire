package logout

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan POST /auth/logout (di bawah /api).
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(NewRepository(d.DB), audit.New(d.Bus, d.Log), s.Session)
	h := NewHandler(uc, s.Session.CookieSecure)
	d.API.With(middleware.RequireAuth()).Post("/auth/logout", h.ServeHTTP)
}
