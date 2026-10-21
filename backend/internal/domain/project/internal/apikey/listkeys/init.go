package listkeys

import (
	"grimoire/internal/domain/project/internal/apikey/base/access"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan GET /api/projects/{projectID}/api-keys.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)))
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-keys", newHandler(uc).ServeHTTP)
}
