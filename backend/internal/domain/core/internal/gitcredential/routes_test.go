package gitcredential

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
)

// Route yang terpasang persis ini: PUT/GET/DELETE di /me/git-credentials, semuanya RequireAuth.
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
		"DELETE /me/git-credentials/{provider}",
		"GET /me/git-credentials",
		"PUT /me/git-credentials/{provider}",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("route tidak sesuai\ngot:  %q\nwant: %q", got, want)
	}
	for _, r := range want {
		parts := strings.SplitN(r, " ", 2)
		target := strings.ReplaceAll(parts[1], "{provider}", "github")
		w := httptest.NewRecorder()
		d.API.ServeHTTP(w, httptest.NewRequest(parts[0], target, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", r, w.Code)
		}
	}
}

// Regresi: GET /me/git-credentials harus sampai ke listcredentials, bukan tertimpa Mount DELETE
// (id non-uuid -> repo mengembalikan daftar kosong tanpa query).
func TestGetListNotShadowedByDeleteGroup(t *testing.T) {
	d := testDeps()
	Register(d)
	r := httptest.NewRequest(http.MethodGet, "/me/git-credentials", nil)
	r = r.WithContext(reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: "bukan-uuid"}))
	w := httptest.NewRecorder()
	d.API.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("GET list: %d %s", w.Code, w.Body.String())
	}
}

// Handler bus get-token terdaftar sekali; registrasi ganda pada registry yang sama = panic.
func TestBusHandlersRegisteredOnce(t *testing.T) {
	d := testDeps()
	Register(d)
	_, err := d.Bus.Call(context.Background(), constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, struct{}{})
	if errors.Is(err, bus.ErrNotRegistered) {
		t.Fatal("core.gitcredential.get-token tidak terdaftar")
	}
	if !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v, want ErrBadRequestType", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Register kedua pada registry yang sama harus panic")
		}
	}()
	Register(d)
}
