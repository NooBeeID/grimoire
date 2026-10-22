package apispec

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apispec/getendpoint"
	"grimoire/lib/contracts/project/apispec/listendpoints"
	"grimoire/lib/contracts/project/apispec/listspecs"
	"grimoire/lib/contracts/project/apispec/upsertspecs"
	"grimoire/lib/contracts/project/management/checkaccess"
	"grimoire/lib/contracts/project/management/contentcount"
)

// emptyDriver adalah driver database/sql yang selalu mengembalikan nol baris, sehingga handler
// dapat dijalankan tanpa Postgres (spec tidak pernah ditemukan).
type emptyDriver struct{}

func (emptyDriver) Open(string) (driver.Conn, error) { return emptyConn{}, nil }

type emptyConn struct{}

func (emptyConn) Prepare(string) (driver.Stmt, error) { return emptyStmt{}, nil }
func (emptyConn) Close() error                        { return nil }
func (emptyConn) Begin() (driver.Tx, error)           { return nil, errors.New("tx tidak didukung") }

type emptyStmt struct{}

func (emptyStmt) Close() error                               { return nil }
func (emptyStmt) NumInput() int                              { return -1 }
func (emptyStmt) Exec([]driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }
func (emptyStmt) Query([]driver.Value) (driver.Rows, error)  { return emptyRows{}, nil }

type emptyRows struct{}

func (emptyRows) Columns() []string         { return nil }
func (emptyRows) Close() error              { return nil }
func (emptyRows) Next([]driver.Value) error { return io.EOF }

func init() { sql.Register("apispec-empty", emptyDriver{}) }

const routeProject = "0b6c0000-0000-4000-8000-0000000000aa"

func newRoutesEnv(t *testing.T) (chi.Router, *bus.Registry, *atomic.Int32) {
	t.Helper()
	raw, err := sql.Open("apispec-empty", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var access atomic.Int32
	reg.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, func(_ context.Context, req any) (any, error) {
		access.Add(1)
		return checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelMember}, nil
	})
	api := chi.NewRouter()
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: "0b6c0000-0000-4000-8000-000000000001", Email: "a@b.c", Name: "A", Role: "developer"})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	Register(app.Deps{
		Config: config.Config{MaxUploadBytes: 1 << 20, MaxUploadFiles: 10, APISpecMaxFileBytes: 1 << 20, APISpecMaxSpecs: 10},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     &db.DB{DB: sqlx.NewDb(raw, "pgx")},
		Bus:    reg,
		API:    api,
	})
	return api, reg, &access
}

func TestRoutesRegistered(t *testing.T) {
	api, reg, _ := newRoutesEnv(t)

	var got []string
	err := chi.Walk(api, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{
		"DELETE /projects/{projectID}/api-specs/{name}",
		"GET /projects/{projectID}/api-specs",
		"GET /projects/{projectID}/api-specs/{name}",
		"GET /projects/{projectID}/api-specs/{name}/raw",
		"PATCH /projects/{projectID}/api-specs/{name}",
		"POST /projects/{projectID}/api-specs/upload",
		"POST /projects/{projectID}/api-specs/validate",
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("route tidak sesuai\ngot:  %q\nwant: %q", got, want)
	}

	ctx := context.Background()
	calls := map[string]any{
		constants.BUS_PROJECT_APISPEC_COUNT_SPECS:    contentcount.Request{ProjectIDs: []string{routeProject}},
		constants.BUS_PROJECT_APISPEC_LIST_SPECS:     listspecs.Request{ProjectID: routeProject},
		constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS: listendpoints.Request{ProjectID: routeProject},
		constants.BUS_PROJECT_APISPEC_GET_ENDPOINT:   getendpoint.Request{ProjectID: routeProject, Spec: "x", Method: "GET", Path: "/p"},
		constants.BUS_PROJECT_APISPEC_UPSERT_SPECS:   upsertspecs.Request{},
	}
	for ep, req := range calls {
		if _, err := reg.Call(ctx, ep, req); errors.Is(err, bus.ErrNotRegistered) {
			t.Errorf("endpoint bus %s tidak terdaftar", ep)
		}
	}
}

func doReq(h http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

// GET .../upload dan .../validate jatuh ke getspec ({name}), bukan 405; POST .../{name} -> 405.
func TestRoutesRegistered_StaticSegmentsFallThroughToGetSpec(t *testing.T) {
	api, _, access := newRoutesEnv(t)
	base := "/projects/" + routeProject + "/api-specs/"

	for _, name := range []string{"upload", "validate"} {
		w := doReq(api, http.MethodGet, base+name)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "spec tidak ditemukan") {
			t.Fatalf("GET %s: want 404 dari getspec, got %d %s", name, w.Code, w.Body.String())
		}
	}
	if access.Load() != 2 {
		t.Fatalf("check-access dipanggil %d kali, want 2", access.Load())
	}

	// Pembanding: daftar spec tetap 200 (listspecs), bukan getspec.
	if w := doReq(api, http.MethodGet, "/projects/"+routeProject+"/api-specs"); w.Code != http.StatusOK {
		t.Fatalf("GET list: %d %s", w.Code, w.Body.String())
	}

	for _, name := range []string{"payment", "x"} {
		if w := doReq(api, http.MethodPost, base+name); w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s: want 405, got %d", name, w.Code)
		}
	}
	// PATCH/DELETE pada nama "upload" tetap sampai ke handler {name} (bukan 405).
	for _, m := range []string{http.MethodPatch, http.MethodDelete} {
		if w := doReq(api, m, base+"upload"); w.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s upload: tidak boleh 405", m)
		}
	}
}

// Registrasi bus ganda pada registry yang sama harus panic (handler terdaftar sekali).
func TestRegister_DoubleBusRegistrationPanics(t *testing.T) {
	api, reg, _ := newRoutesEnv(t)
	_ = api
	defer func() {
		if recover() == nil {
			t.Fatal("Register kedua pada registry yang sama harus panic")
		}
	}()
	raw, err := sql.Open("apispec-empty", "")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	Register(app.Deps{
		Config: config.Config{MaxUploadBytes: 1 << 20, MaxUploadFiles: 10, APISpecMaxFileBytes: 1 << 20, APISpecMaxSpecs: 10},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     &db.DB{DB: sqlx.NewDb(raw, "pgx")},
		Bus:    reg,
		API:    chi.NewRouter(),
	})
}
