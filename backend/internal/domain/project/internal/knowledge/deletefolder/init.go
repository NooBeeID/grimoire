package deletefolder

import (
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang DELETE /projects/{projectID}/knowledge/folders pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)), audit.New(d.Bus, d.Log))
	h := &handler{uc: uc}
	d.API.With(middleware.RequireAuth()).
		Delete("/projects/{projectID}/knowledge/folders", h.serve)
}
