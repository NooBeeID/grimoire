package search

import (
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/searchrun"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang GET /projects/{projectID}/knowledge/search pada d.API.
func Init(d app.Deps) {
	run := searchrun.New(store.New(d.DB), searchrun.NewBusSynonyms(d.Bus), d.Log)
	uc := NewUsecase(access.New(d.Bus), run)
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/knowledge/search", newHandler(uc).ServeHTTP)
}
