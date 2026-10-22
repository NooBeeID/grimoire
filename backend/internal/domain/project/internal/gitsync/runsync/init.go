package runsync

import (
	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/base/access"
	"grimoire/internal/domain/project/internal/gitsync/base/audit"
	"grimoire/internal/domain/project/internal/gitsync/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan POST /api/projects/{projectID}/git-sync/run.
func Init(d app.Deps, sh base.Shared) {
	clock := sh.Clock
	if clock == nil {
		clock = base.SystemClock
	}
	uc := NewUsecase(access.New(d.Bus), newRepo(d.DB, store.New(d.DB)), sh.Git, d.Bus,
		audit.New(d.Bus, d.Log), clock, d.Config, d.Log)
	d.API.With(
		middleware.RequireAuth(),
		middleware.BodyLimit(16<<10),
		middleware.ExtendTimeout(d.Config.GitSyncTimeout),
	).Post("/projects/{projectID}/git-sync/run", newHandler(uc).ServeHTTP)
}
