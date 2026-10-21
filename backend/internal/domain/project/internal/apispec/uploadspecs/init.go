package uploadspecs

import (
	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang POST /projects/{projectID}/api-specs/upload pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	clock := base.Clock(base.SystemClock)
	uc := NewUsecase(newRepo(d.DB, s), access.New(d.Bus), audit.New(d.Bus, d.Log, clock), clock,
		d.Config.APISpecMaxSpecs, d.Config.APISpecMaxFileBytes)
	h := &Handler{uc: uc, maxFileBytes: d.Config.APISpecMaxFileBytes, maxFiles: d.Config.MaxUploadFiles}
	d.API.With(middleware.RequireAuth(), middleware.BodyLimit(d.Config.MaxUploadBytes)).
		Post("/projects/{projectID}/api-specs/upload", h.ServeHTTP)
}
