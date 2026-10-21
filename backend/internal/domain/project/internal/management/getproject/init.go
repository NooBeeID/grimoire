package getproject

import (
	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang GET /projects/{projectID} pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(d.DB, s), counts.New(d.Bus, d.Log))
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}", newHandler(uc).ServeHTTP)
}
