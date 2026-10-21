package inviteuser

import (
	"strings"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init memasang POST /projects/{projectID}/invitations pada d.API.
func Init(d app.Deps) {
	s := store.New(d.DB)
	uc := NewUsecase(newRepo(d.DB, s), users.New(d.Bus), audit.New(d.Bus, d.Log), base.SystemClock, d.Log)
	d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/invitations", newHandler(uc).ServeHTTP)
}

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
