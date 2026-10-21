package getfile

import (
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan GET /api/projects/{projectID}/knowledge/files.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), store.New(d.DB))
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/knowledge/files", NewHandler(uc).Handle)
}
