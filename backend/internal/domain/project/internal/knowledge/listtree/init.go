package listtree

import (
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang GET /projects/{projectID}/knowledge/tree pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), store.New(d.DB))
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/knowledge/tree", newHandler(uc).ServeHTTP)
}
