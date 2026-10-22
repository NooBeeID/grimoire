package gitcredential

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
)

func TestRegister_Idempotent(t *testing.T) {
	d := testDeps()
	Register(d)
	Register(testDeps()) // kode error tidak boleh didaftarkan dua kali
	if newShared(d).Enabled || newShared(d).Box != nil {
		t.Fatal("tanpa kunci fitur nonaktif")
	}
	d.Config.GitCredentialKey = make([]byte, 32)
	sh := newShared(d)
	if !sh.Enabled || sh.Box == nil || sh.Git == nil {
		t.Fatal("dengan kunci fitur aktif")
	}
}

func testDeps() app.Deps {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return app.Deps{
		Ctx:    context.Background(),
		Config: config.Config{},
		Log:    log,
		Bus:    bus.NewRegistry(log),
		API:    chi.NewRouter(),
	}
}

// Semua route terpasang dan dilindungi RequireAuth (tanpa sesi -> 401, bukan 404/405).
func TestRegister_Routes(t *testing.T) {
	d := testDeps()
	Register(d)
	for _, rt := range [][2]string{
		{http.MethodGet, "/me/git-credentials"},
		{http.MethodPut, "/me/git-credentials/github"},
		{http.MethodDelete, "/me/git-credentials/github"},
	} {
		rec := httptest.NewRecorder()
		d.API.ServeHTTP(rec, httptest.NewRequest(rt[0], rt[1], nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d want 401", rt[0], rt[1], rec.Code)
		}
	}
}
