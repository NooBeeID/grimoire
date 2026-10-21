package deleteproject

import (
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang DELETE /projects/{projectID} pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(d.DB, s), counts.New(d.Bus, d.Log), audit.New(d.Bus, d.Log))
	h := &Handler{uc: uc}
	d.API.With(middleware.RequireAuth()).Delete("/projects/{projectID}", h.ServeHTTP)
}
