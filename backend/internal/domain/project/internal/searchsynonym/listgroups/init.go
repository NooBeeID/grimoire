package listgroups

import (
	"grimoire/internal/domain/project/internal/searchsynonym/base/access"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang GET /projects/{projectID}/search/synonyms pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)))
	h := &handler{uc: uc}
	d.API.With(middleware.RequireAuth()).
		Get("/projects/{projectID}/search/synonyms", h.serve)
}
