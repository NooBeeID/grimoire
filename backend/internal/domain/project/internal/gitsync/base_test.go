package gitsync

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/http/respond"
	"grimoire/lib/common/errs"
)

func TestRegister_Idempotent(t *testing.T) {
	d := testDeps()
	Register(d)
	Register(testDeps())
	for code, want := range map[errs.Code]int{
		"CREDENTIAL_MISSING": 422, "CREDENTIAL_UNREADABLE": 422, "INVALID_REPO_URL": 400, "GIT_TOKEN_REJECTED": 422,
		"GIT_ACCESS_DENIED": 422, "GIT_NOT_FOUND": 404, "SYNC_LIMIT_EXCEEDED": 422, "SYNC_IN_PROGRESS": 409,
		"GIT_TREE_TOO_LARGE": 422, errs.CodeHostNotAllowed: 422, errs.CodeGitUpstream: 502, errs.CodeGitFeatureDisabled: 503,
	} {
		if got := respond.StatusFor(code); got != want {
			t.Errorf("%s: %d want %d", code, got, want)
		}
	}
}

func testDeps() app.Deps {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return app.Deps{
		Ctx:    context.Background(),
		Config: config.Config{GitSyncTimeout: time.Minute},
		Log:    log,
		Bus:    bus.NewRegistry(log),
		API:    chi.NewRouter(),
	}
}

// Route terpasang dan dilindungi RequireAuth (tanpa sesi -> 401, bukan 404/405).
func TestRegister_Routes(t *testing.T) {
	d := testDeps()
	Register(d)
	for _, rt := range [][2]string{
		{http.MethodGet, "/projects/0b6c0000-0000-4000-8000-0000000000aa/git-sync"},
		{http.MethodPost, "/projects/0b6c0000-0000-4000-8000-0000000000aa/git-sync/run"},
	} {
		rec := httptest.NewRecorder()
		d.API.ServeHTTP(rec, httptest.NewRequest(rt[0], rt[1], nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d want 401", rt[0], rt[1], rec.Code)
		}
	}
}
