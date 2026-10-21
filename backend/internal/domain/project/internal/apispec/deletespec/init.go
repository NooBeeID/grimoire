package deletespec

import (
	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang DELETE /projects/{projectID}/api-specs/{name} pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(d.DB, s), access.New(d.Bus), audit.New(d.Bus, d.Log, base.Clock(base.SystemClock)))
	h := newHandler(uc)
	d.API.With(middleware.RequireAuth()).Delete("/projects/{projectID}/api-specs/{name}", h.ServeHTTP)
}
