package downloadspec

import (
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan GET /api/projects/{projectID}/api-specs/{name}/raw.
func Init(d app.Deps) {
	uc := NewUsecase(store.New(d.DB), access.New(d.Bus))
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-specs/{name}/raw", newHandler(uc, d.Log).ServeHTTP)
}
