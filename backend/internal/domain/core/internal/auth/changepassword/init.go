package changepassword

import (
	"golang.org/x/time/rate"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan POST /auth/change-password (di bawah /api) dengan rate limit per user.
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(NewRepo(d.DB), s.Hasher, audit.New(d.Bus, d.Log), s.Password, s.Clock)
	h := NewHandler(uc, s.Session.Secret)
	n := s.LoginRatePerMinute
	lim := middleware.NewLimiter(d.Ctx, middleware.Rule{
		Name:    "auth.change-password",
		Rate:    rate.Limit(float64(n) / 60),
		Burst:   max(1, n/4),
		KeyFunc: middleware.KeyByUser,
	})
	d.API.With(middleware.RequireAuth(), middleware.RateLimit(lim)).Post("/auth/change-password", h.ChangePassword)
}
