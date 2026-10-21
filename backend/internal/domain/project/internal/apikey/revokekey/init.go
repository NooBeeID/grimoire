package revokekey

import (
	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/access"
	"grimoire/internal/domain/project/internal/apikey/base/audit"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang POST /projects/{projectID}/api-keys/{keyID}/revoke pada d.API.
func Init(d app.Deps) {
	uc := NewUsecase(newRepo(d.DB, store.New(d.DB)), access.New(d.Bus), audit.New(d.Bus, d.Log), base.SystemClock)
	d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/api-keys/{keyID}/revoke", newHandler(uc).ServeHTTP)
}
