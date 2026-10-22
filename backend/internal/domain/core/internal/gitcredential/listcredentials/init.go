package listcredentials

import (
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/gitprovider"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan GET /api/me/git-credentials (path penuh, bukan Route group).
func Init(d app.Deps) {
	policy := gitprovider.HostPolicy{AllowedGitLabHosts: d.Config.GitSyncAllowedHosts}
	uc := NewUsecase(newRepo(d.DB), Settings{
		Enabled:     d.Config.GitFeatureEnabled(),
		GitLabHosts: policy.GitLabHosts(),
	})
	d.API.With(middleware.RequireAuth()).Get("/me/git-credentials", newHandler(uc).ServeHTTP)
}
