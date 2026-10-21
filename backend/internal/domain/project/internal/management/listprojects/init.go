package listprojects

import (
	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang GET /projects pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(NewRepo(d.DB), counts.New(d.Bus, d.Log))
	d.API.With(middleware.RequireAuth()).Get("/projects", newHandler(uc).ServeHTTP)
}
