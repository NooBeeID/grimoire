package movefile

import (
	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang POST /projects/{projectID}/knowledge/files/move pada d.API.
func Init(d app.Deps) {
	limits := base.LimitsFrom(d.Config)
	uc := NewUsecase(access.New(d.Bus), NewRepo(store.New(d.DB)), audit.New(d.Bus, d.Log))
	h := NewHandler(uc)
	d.API.With(middleware.RequireAuth(), middleware.BodyLimit(limits.JSONBodyLimit())).
		Post("/projects/{projectID}/knowledge/files/move", h.Handle)
}
