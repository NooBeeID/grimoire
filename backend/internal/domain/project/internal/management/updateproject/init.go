package updateproject

import (
	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang PATCH /projects/{projectID} pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(d.DB, s), audit.New(d.Bus, d.Log), counts.New(d.Bus, d.Log), base.SystemClock)
	d.API.With(middleware.RequireAuth()).Patch("/projects/{projectID}", handler{uc: uc}.ServeHTTP)
}
