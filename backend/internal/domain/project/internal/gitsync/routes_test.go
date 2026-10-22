package gitsync

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const routeProject = "0b6c0000-0000-4000-8000-0000000000aa"

// Route persis dua, keduanya RequireAuth. gitsync adalah consumer bus murni: tidak mendaftarkan handler bus.
func TestRoutesRegisteredWithAuth(t *testing.T) {
	d := testDeps()
	Register(d)

	var got []string
	if err := chi.Walk(d.API, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{
		"GET /projects/{projectID}/git-sync",
		"POST /projects/{projectID}/git-sync/run",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("route tidak sesuai\ngot:  %q\nwant: %q", got, want)
	}
	for _, r := range want {
		parts := strings.SplitN(r, " ", 2)
		target := strings.ReplaceAll(parts[1], "{projectID}", routeProject)
		w := httptest.NewRecorder()
		d.API.ServeHTTP(w, httptest.NewRequest(parts[0], target, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", r, w.Code)
		}
	}
}

// Module ini hanya memanggil bus, tidak menyediakan endpoint: Register tidak boleh panic walau
// dipasang dua kali pada registry yang sama (tidak ada registrasi handler bus).
func TestNoBusHandlers_RegisterTwiceOnSameRegistryDoesNotPanic(t *testing.T) {
	d := testDeps()
	Register(d)
	Register(d)
}
