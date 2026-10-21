package validatespec

import (
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
)

func TestInit_DoesNotTouchDB(t *testing.T) {
	r := chi.NewRouter()
	Init(app.Deps{DB: nil, Bus: bus.NewRegistry(slog.Default()), API: r,
		Config: config.Config{APISpecMaxFileBytes: 1024, MaxUploadFiles: 5, MaxUploadBytes: 1 << 20}})
	found := false
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodPost && route == "/projects/{projectID}/api-specs/validate" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("route tidak terdaftar")
	}
}

func TestValidatespec_ImportsNoPersistence(t *testing.T) {
	banned := []string{"internal/infra/db", "github.com/jmoiron/sqlx", "apispec/base/store", "apispec/base/audit",
		"database/sql", "net/http/httputil", "grimoire/internal/infra/bus"}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range af.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			for _, b := range banned {
				if strings.Contains(p, b) {
					t.Errorf("%s mengimpor %s", f, p)
				}
			}
		}
	}
	if _, err := os.Stat("repository.go"); err == nil {
		t.Error("repository.go tidak boleh ada")
	}
}
