package savecredential

import (
	"golang.org/x/time/rate"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

const (
	saveRatePerMinute = 10
	saveBurst         = 5
	maxBodyBytes      = 8 << 10
)

// Init memasang PUT /me/git-credentials/{provider} pada d.API.
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(
		s.Enabled,
		s.Git.Policy(),
		s.Git,
		sealerOf(s),
		newRepo(d.DB),
		audit.New(d.Bus, d.Log),
		s.Clock,
		d.Log,
	)
	lim := middleware.NewLimiter(d.Ctx, middleware.Rule{
		Name: "gitcredential.save", Rate: rate.Limit(float64(saveRatePerMinute) / 60),
		Burst: saveBurst, KeyFunc: middleware.KeyByUser,
	})
	d.API.With(
		middleware.RequireAuth(),
		middleware.BodyLimit(maxBodyBytes),
		middleware.RateLimit(lim),
	).Put("/me/git-credentials/{provider}", newHandler(uc).ServeHTTP)
}

// sealerOf menghindari nil-pointer dalam interface: nil interface bila Box nil.
func sealerOf(s base.Shared) Sealer {
	if s.Box == nil {
		return nil
	}
	return s.Box
}
