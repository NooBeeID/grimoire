package knowledge

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/jobs"
	"grimoire/lib/constants"
)

const routeProject = "0b6c0000-0000-4000-8000-0000000000aa"

func newRoutesEnv() (app.Deps, chi.Router, *bus.Registry) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := bus.NewRegistry(log)
	api := chi.NewRouter()
	d := app.Deps{
		Ctx:    context.Background(),
		Config: config.Config{MaxUploadBytes: 1 << 20, MaxUploadFiles: 10, MaxFileBytes: 1 << 20},
		Log:    log,
		Bus:    reg,
		API:    api,
		Jobs:   jobs.New(nil, log),
	}
	return d, api, reg
}

var wantRoutes = []string{
	"DELETE /projects/{projectID}/knowledge/files",
	"DELETE /projects/{projectID}/knowledge/folders",
	"GET /projects/{projectID}/knowledge/files",
	"GET /projects/{projectID}/knowledge/search",
	"GET /projects/{projectID}/knowledge/tree",
	"POST /projects/{projectID}/knowledge/files",
	"POST /projects/{projectID}/knowledge/files/move",
	"POST /projects/{projectID}/knowledge/upload",
	"PUT /projects/{projectID}/knowledge/files",
}

func TestRoutesRegisteredWithAuth(t *testing.T) {
	d, api, _ := newRoutesEnv()
	Register(d)

	var got []string
	if err := chi.Walk(api, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := append([]string(nil), wantRoutes...)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("route tidak sesuai\ngot:  %q\nwant: %q", got, want)
	}

	// Tanpa sesi semua route harus 401 (RequireAuth terpasang sebelum handler/akses project).
	for _, r := range want {
		parts := strings.SplitN(r, " ", 2)
		target := strings.ReplaceAll(parts[1], "{projectID}", routeProject)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequest(parts[0], target, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", r, w.Code)
		}
	}
}

// Setiap endpoint bus milik module terdaftar tepat sekali; registrasi ganda = panic.
func TestBusHandlersRegisteredOnce(t *testing.T) {
	d, _, reg := newRoutesEnv()
	Register(d)

	for _, ep := range []string{
		constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
		constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
		constants.BUS_PROJECT_KNOWLEDGE_READ_FILE,
		constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
		constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES,
	} {
		// Tipe request salah: handler terdaftar menjawab ErrBadRequestType tanpa menyentuh DB.
		_, err := reg.Call(context.Background(), ep, struct{}{})
		if errors.Is(err, bus.ErrNotRegistered) {
			t.Errorf("endpoint bus %s tidak terdaftar", ep)
		} else if !errors.Is(err, bus.ErrBadRequestType) {
			t.Errorf("endpoint bus %s: err=%v, want ErrBadRequestType", ep, err)
		}
	}

	defer func() {
		if recover() == nil {
			t.Fatal("Register kedua pada registry yang sama harus panic (registrasi bus ganda)")
		}
	}()
	Register(d)
}
