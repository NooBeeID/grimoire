package renamespec

import (
	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang PATCH /api/projects/{projectID}/api-specs/{name}.
func Init(d app.Deps) {
	clock := base.Clock(base.SystemClock)
	uc := NewUsecase(newRepo(d.DB, store.New(d.DB)), access.New(d.Bus), audit.New(d.Bus, d.Log, clock), clock)
	d.API.With(middleware.RequireAuth()).Patch("/projects/{projectID}/api-specs/{name}", newHandler(uc).ServeHTTP)
}
