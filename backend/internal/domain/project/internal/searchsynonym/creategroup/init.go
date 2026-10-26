package creategroup

import (
	"grimoire/internal/domain/project/internal/searchsynonym/base/access"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// BodyLimitBytes adalah batas body POST/PUT kamus sinonim.
const BodyLimitBytes = 8 << 10

// Init memasang POST /projects/{projectID}/search/synonyms pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)), audit.New(d.Bus, d.Log))
	h := &handler{uc: uc}
	d.API.With(middleware.RequireAuth(), middleware.BodyLimit(BodyLimitBytes)).
		Post("/projects/{projectID}/search/synonyms", h.serve)
}
