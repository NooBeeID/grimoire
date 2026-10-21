package listspecs

import (
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan GET /api/projects/{projectID}/api-specs.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), store.New(d.DB), d.Config.APISpecMaxSpecs)
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-specs", newHandler(uc).ServeHTTP)
}
