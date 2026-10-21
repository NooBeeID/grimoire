package validatespec

import (
	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang POST /projects/{projectID}/api-specs/validate pada d.API. Tidak menyentuh d.DB.
func Init(d app.Deps) {
	limits := base.UploadLimits{MaxFileBytes: d.Config.APISpecMaxFileBytes, MaxFiles: d.Config.MaxUploadFiles}
	uc := NewUsecase(access.New(d.Bus), limits)
	h := newHandler(uc, limits.MaxFileBytes, limits.MaxFiles)
	d.API.With(middleware.RequireAuth(), middleware.BodyLimit(d.Config.MaxUploadBytes)).
		Post("/projects/{projectID}/api-specs/validate", h.ServeHTTP)
}
