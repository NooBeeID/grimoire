package deletecredential

import (
	"grimoire/internal/domain/core/internal/gitcredential/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan DELETE /api/me/git-credentials/{provider} (path penuh di d.API, bukan
// Route group: Mount pada /me/git-credentials akan menimpa GET /me/git-credentials milik listcredentials).
func Init(d app.Deps) {
	uc := NewUsecase(newRepo(d.DB), audit.New(d.Bus, d.Log))
	d.API.With(middleware.RequireAuth()).Delete("/me/git-credentials/{provider}", newHandler(uc).ServeHTTP)
}
