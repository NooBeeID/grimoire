package getconfig

import (
	"grimoire/internal/domain/project/internal/gitsync/base/access"
	"grimoire/internal/domain/project/internal/gitsync/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan GET /api/projects/{projectID}/git-sync.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)),
		busCredentials{b: d.Bus}, d.Config.GitFeatureEnabled(), d.Log)
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/git-sync", newHandler(uc).ServeHTTP)
}
