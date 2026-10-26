package deletegroup

import (
	"grimoire/internal/domain/project/internal/searchsynonym/base/access"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang DELETE /projects/{projectID}/search/synonyms/{groupID} pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)), audit.New(d.Bus, d.Log))
	h := &handler{uc: uc}
	d.API.With(middleware.RequireAuth()).
		Delete("/projects/{projectID}/search/synonyms/{groupID}", h.serve)
}
