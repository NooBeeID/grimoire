// Package gitsync adalah module project/git-sync: menarik file dari repo GitHub/GitLab (REST, PAT
// milik user pemanggil) ke knowledge atau API spec lewat bus.
//
// Register mendaftarkan kode error, merakit Shared, lalu satu Init per feature (getconfig,
// runsync). Route memakai pola penuh di d.API sesuai konvensi /projects:
//
//	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/git-sync", ...)
//	d.API.With(middleware.RequireAuth(), middleware.BodyLimit(16<<10),
//	    middleware.ExtendTimeout(cfg.GitSyncTimeout+30*time.Second)).Post("/projects/{projectID}/git-sync/run", ...)
//
// ExtendTimeout dipasang di runsync/init.go. Route run melayani JSON atau SSE (Accept:
// text/event-stream); git.Client tetap *gitprovider.Client (pacing dari config lewat
// NewFromConfig), runsync membungkusnya dengan adapter Session.
package gitsync

import (
	"net/http"
	"sync"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/getconfig"
	"grimoire/internal/domain/project/internal/gitsync/runsync"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/gitprovider"
	httpx "grimoire/internal/infra/http"
	"grimoire/lib/common/errs"
)

var codesOnce sync.Once

// registerCodes mendaftarkan status HTTP kode error milik module. sync.Once karena
// RegisterCodeStatus panic bila duplikat. Kode bersama (HOST_NOT_ALLOWED, GIT_UPSTREAM_ERROR,
// GIT_FEATURE_DISABLED, RATE_LIMITED, TIMEOUT) sudah ada di peta bawaan.
func registerCodes() {
	codesOnce.Do(func() {
		for code, status := range map[errs.Code]int{
			base.CodeCredentialMissing:    http.StatusUnprocessableEntity,
			base.CodeCredentialUnreadable: http.StatusUnprocessableEntity,
			base.CodeInvalidRepoURL:       http.StatusBadRequest,
			base.CodeGitTokenRejected:     http.StatusUnprocessableEntity,
			base.CodeGitAccessDenied:      http.StatusUnprocessableEntity,
			base.CodeGitNotFound:          http.StatusNotFound,
			base.CodeSyncLimitExceeded:    http.StatusUnprocessableEntity,
			base.CodeSyncInProgress:       http.StatusConflict,
			base.CodeGitTreeTooLarge:      http.StatusUnprocessableEntity,
		} {
			httpx.RegisterCodeStatus(code, status)
		}
	})
}

// newShared merakit dependensi bersama dari config.
func newShared(d app.Deps) base.Shared {
	return base.Shared{Git: gitprovider.NewFromConfig(d.Config), Clock: base.SystemClock}
}

// Register merakit module.
func Register(d app.Deps) {
	registerCodes()
	sh := newShared(d)
	getconfig.Init(d)
	runsync.Init(d, sh)
}
