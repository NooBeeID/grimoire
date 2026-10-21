package searchmembercandidates

import (
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang GET /projects/{projectID}/member-candidates pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(s), users.New(d.Bus))
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/member-candidates", newHandler(uc).ServeHTTP)
}
