package addmember

import (
	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang POST /projects/{projectID}/members pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(d.DB, s), users.New(d.Bus), audit.New(d.Bus, d.Log), base.SystemClock)
	d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/members", newHandler(uc).ServeHTTP)
}
