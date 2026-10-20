package login

import (
	"golang.org/x/time/rate"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init merakit usecase dan mendaftarkan POST /auth/login (relatif terhadap d.API) dengan rate limit per IP.
func Init(d app.Deps, s base.Shared) {
	repo := NewRepository(d.DB)
	uc := NewUsecase(repo, s, audit.New(d.Bus, d.Log), d.Log)
	h := NewHandler(uc, s.Session.CookieSecure)

	n := s.LoginRatePerMinute
	limiter := middleware.NewLimiter(d.Ctx, middleware.Rule{
		Name:    "auth.login",
		Rate:    rate.Limit(float64(n) / 60),
		Burst:   max(1, n/4),
		KeyFunc: middleware.KeyByIP,
	})
	d.API.With(middleware.RateLimit(limiter)).Post("/auth/login", h.Login)
}
