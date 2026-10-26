package searchsynonym_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/searchsynonym/creategroup"
	"grimoire/internal/domain/project/internal/searchsynonym/deletegroup"
	"grimoire/internal/domain/project/internal/searchsynonym/listgroups"
	"grimoire/internal/domain/project/internal/searchsynonym/updategroup"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
)

const (
	routeProject = "0b6c0000-0000-4000-8000-0000000000aa"
	routeGroup   = "0b6c0000-0000-4000-8000-0000000000bb"
)

var wantRoutes = []string{
	"DELETE /projects/{projectID}/search/synonyms/{groupID}",
	"GET /projects/{projectID}/search/synonyms",
	"POST /projects/{projectID}/search/synonyms",
	"PUT /projects/{projectID}/search/synonyms/{groupID}",
}

// Route dibangun lewat Init tiap feature (base.go dirakit fase wiring).
func TestRoutesRegisteredWithAuth(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	api := chi.NewRouter()
	d := app.Deps{Ctx: context.Background(), Log: log, Bus: bus.NewRegistry(log), API: api}
	listgroups.Init(d)
	creategroup.Init(d)
	updategroup.Init(d)
	deletegroup.Init(d)

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

	// Tanpa sesi semua route harus 401 (RequireAuth sebelum handler/akses project).
	for _, r := range want {
		parts := strings.SplitN(r, " ", 2)
		target := strings.NewReplacer("{projectID}", routeProject, "{groupID}", routeGroup).Replace(parts[1])
		body := strings.NewReader("")
		if parts[0] == http.MethodPost || parts[0] == http.MethodPut {
			body = strings.NewReader(`{"terms":["a","b"]}`)
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequest(parts[0], target, body))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", r, w.Code)
		}
	}
}
