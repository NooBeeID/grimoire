package updategroup

import (
	"grimoire/internal/domain/project/internal/searchsynonym/base/access"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// bodyLimitBytes adalah batas body PUT kamus sinonim.
const bodyLimitBytes = 8 << 10

// Init memasang PUT /projects/{projectID}/search/synonyms/{groupID} pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)), audit.New(d.Bus, d.Log))
	h := &handler{uc: uc}
	d.API.With(middleware.RequireAuth(), middleware.BodyLimit(bodyLimitBytes)).
		Put("/projects/{projectID}/search/synonyms/{groupID}", h.serve)
}
