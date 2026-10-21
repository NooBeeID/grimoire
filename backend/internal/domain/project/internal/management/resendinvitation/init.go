package resendinvitation

import (
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang POST /projects/{projectID}/invitations/{userID}/resend pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(s), users.New(d.Bus), d.Log)
	d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/invitations/{userID}/resend", handler{uc: uc}.ServeHTTP)
}
