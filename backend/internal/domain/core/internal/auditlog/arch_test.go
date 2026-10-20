package auditlog

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/jobs"
)

var (
	reUpdate   = regexp.MustCompile(`(?i)\bupdate\s+audit_log\b`)
	reDelete   = regexp.MustCompile(`(?i)\bdelete\s+from\s+audit_log\b`)
	reTruncate = regexp.MustCompile(`(?i)\btruncate\b.*audit_log`)
)

// AC7: tidak ada kode produksi (bukan _test.go; tes boleh membersihkan tabel) yang meng-update/menghapus entri selain purgeexpired.
func TestAppendOnlySources(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(b)
		if reUpdate.MatchString(src) {
			t.Errorf("%s: UPDATE audit_log dilarang (append-only)", path)
		}
		if reTruncate.MatchString(src) {
			t.Errorf("%s: TRUNCATE audit_log dilarang", path)
		}
		if reDelete.MatchString(src) && filepath.ToSlash(path) != "purgeexpired/repository.go" {
			t.Errorf("%s: DELETE FROM audit_log hanya boleh di purgeexpired/repository.go", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// AC7: hanya dua rute GET di bawah /audit-log.
func TestOnlyGetRoutes(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	api := chi.NewRouter()
	Register(app.Deps{
		Ctx:    context.Background(),
		Config: config.Config{AuditRetentionDays: 90},
		Log:    log,
		Bus:    bus.NewRegistry(log),
		// Scheduler tanpa Start: Register hanya mendaftarkan job.
		Jobs: jobs.New(nil, log),
		API:  api,
		MCP:  chi.NewRouter(),
	})

	var routes []string
	err := chi.Walk(api, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/audit-log") {
			return nil
		}
		if method != http.MethodGet && method != http.MethodHead {
			t.Errorf("rute %s %s: hanya GET yang boleh", method, route)
		}
		if method == http.MethodGet {
			routes = append(routes, route)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 {
		t.Fatalf("rute GET /audit-log = %v, ingin tepat 2", routes)
	}
}
