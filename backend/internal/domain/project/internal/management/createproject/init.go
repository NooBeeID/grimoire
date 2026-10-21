package createproject

import (
	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang POST /projects pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(newRepo(d.DB), audit.New(d.Bus, d.Log), base.SystemClock)
	d.API.With(middleware.RequireAuth()).Post("/projects", newHandler(uc).ServeHTTP)
}
