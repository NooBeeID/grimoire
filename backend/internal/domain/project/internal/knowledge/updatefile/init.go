package updatefile

import (
	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang PUT /projects/{projectID}/knowledge/files pada d.API.
func Init(d app.Deps) {
	limits := base.LimitsFrom(d.Config)
	uc := NewUsecase(NewRepo(store.New(d.DB)), access.New(d.Bus), audit.New(d.Bus, d.Log), limits)
	h := &Handler{uc: uc, log: d.Log}
	d.API.With(middleware.RequireAuth(), middleware.BodyLimit(limits.JSONBodyLimit())).
		Put("/projects/{projectID}/knowledge/files", h.ServeHTTP)
}
