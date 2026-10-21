package changememberrole

import (
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang PATCH /projects/{projectID}/members/{userID} pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(d.DB, s), users.New(d.Bus), audit.New(d.Bus, d.Log), d.Log)
	d.API.With(middleware.RequireAuth()).Patch("/projects/{projectID}/members/{userID}", handler{uc: uc}.ServeHTTP)
}
