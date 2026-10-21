//go:build integration

package listprojects

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/contentcount"
)

type httpEnv struct {
	s *seedData
	b *bus.Registry
	r chi.Router
}

func newHTTPEnv(t *testing.T) *httpEnv {
	t.Helper()
	s := seedProjects(t)
	b := bus.NewRegistry(nil)
	uc := NewUsecase(NewRepo(s.d), counts.New(b, nil))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if id := req.Header.Get("X-Test-User"); id != "" {
				req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: id, Role: req.Header.Get("X-Test-Role")}))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.With(middleware.RequireAuth()).Get("/projects", newHandler(uc).ServeHTTP)
	return &httpEnv{s: s, b: b, r: r}
}

func (e *httpEnv) get(t *testing.T, path, user, role string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if user != "" {
		req.Header.Set("X-Test-User", user)
		req.Header.Set("X-Test-Role", role)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func itemNames(m map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, x := range m["items"].([]any) {
		it := x.(map[string]any)
		out[it["name"].(string)] = it
	}
	return out
}

func TestT03_AC1_ListProjects_DeveloperSeesOnlyOwn(t *testing.T) {
	e := newHTTPEnv(t)
	w, m := e.get(t, "/projects", e.s.A, "developer")
	if w.Code != 200 || m["total"].(float64) != 4 || itemNames(m)["P1"] == nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	_, m = e.get(t, "/projects", e.s.B, "developer")
	if itemNames(m)["P1"] != nil || m["total"].(float64) != 2 {
		t.Fatal("B melihat P1")
	}
}

func TestT03_AC7_ListProjects_SuperAdminSeesAllWithIsMember(t *testing.T) {
	e := newHTTPEnv(t)
	_, m := e.get(t, "/projects", uuid.NewString(), "super_admin")
	if m["total"].(float64) != 7 {
		t.Fatal(m["total"])
	}
	for n, it := range itemNames(m) {
		if it["access_level"] != "admin" || it["is_member"] != false || it["my_role"] != nil {
			t.Fatalf("%s: %v", n, it)
		}
	}
}

func TestT03_ListProjects_RequiresAuth(t *testing.T) {
	e := newHTTPEnv(t)
	w, _ := e.get(t, "/projects", "", "")
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestT03_ListProjects_ValidationErrors(t *testing.T) {
	e := newHTTPEnv(t)
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'a'
	}
	for path, keys := range map[string][]string{
		"/projects?limit=500":         {"limit"},
		"/projects?limit=abc":         {"limit"},
		"/projects?offset=-1":         {"offset"},
		"/projects?q=" + string(long): {"q"},
		"/projects?limit=0&offset=-5": {"limit", "offset"},
	} {
		w, m := e.get(t, path, e.s.A, "developer")
		if w.Code != 400 {
			t.Fatalf("%s: %d", path, w.Code)
		}
		fields := m["error"].(map[string]any)["details"].(map[string]any)["fields"].(map[string]any)
		for _, k := range keys {
			if fields[k] == nil {
				t.Fatalf("%s: %v", path, fields)
			}
		}
	}
}

type provider struct {
	mu    sync.Mutex
	calls [][]string
	files map[string]int
	err   error
}

func (p *provider) handle(_ context.Context, r any) (any, error) {
	req := r.(contentcount.Request)
	p.mu.Lock()
	p.calls = append(p.calls, req.ProjectIDs)
	p.mu.Unlock()
	if p.err != nil {
		return nil, p.err
	}
	out := map[string]int{}
	for _, id := range req.ProjectIDs {
		out[id] = p.files[id]
	}
	return contentcount.Response{Counts: out}, nil
}

func TestT03_Counts_FailSoft_List(t *testing.T) {
	e := newHTTPEnv(t)
	_, m := e.get(t, "/projects", e.s.A, "developer")
	for _, it := range itemNames(m) {
		c := it["counts"].(map[string]any)
		if c["files"] != nil || c["specs"] != nil || c["keys"] != nil {
			t.Fatalf("%v", c)
		}
	}
	p := &provider{files: map[string]int{e.s.p["P1"]: 3}}
	e.b.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, p.handle)
	_, m = e.get(t, "/projects", e.s.A, "developer")
	items := itemNames(m)
	c := items["P1"]["counts"].(map[string]any)
	if c["files"].(float64) != 3 || c["specs"] != nil || c["keys"] != nil {
		t.Fatalf("%v", c)
	}
	if items["P2"]["counts"].(map[string]any)["files"].(float64) != 0 {
		t.Fatal("id tanpa entri harus 0")
	}
}

func TestT03_Counts_ProviderError_List(t *testing.T) {
	e := newHTTPEnv(t)
	ok := &provider{files: map[string]int{}}
	bad := &provider{err: context.DeadlineExceeded}
	e.b.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, ok.handle)
	e.b.Register(constants.BUS_PROJECT_APISPEC_COUNT_SPECS, bad.handle)
	e.b.Register(constants.BUS_PROJECT_APIKEY_COUNT_KEYS, ok.handle)
	w, m := e.get(t, "/projects", e.s.A, "developer")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	c := itemNames(m)["P1"]["counts"].(map[string]any)
	if c["specs"] != nil || c["files"] == nil || c["keys"] == nil {
		t.Fatalf("%v", c)
	}
}

func TestT03_ListProjects_CountsOneCallPerProvider(t *testing.T) {
	e := newHTTPEnv(t)
	for i := 0; i < 20; i++ {
		if _, err := e.s.d.ExecContext(context.Background(), `INSERT INTO projects (name) VALUES ($1)`, "Extra"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	ps := []*provider{{}, {}, {}}
	e.b.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, ps[0].handle)
	e.b.Register(constants.BUS_PROJECT_APISPEC_COUNT_SPECS, ps[1].handle)
	e.b.Register(constants.BUS_PROJECT_APIKEY_COUNT_KEYS, ps[2].handle)
	e.get(t, "/projects", uuid.NewString(), "super_admin")
	for i, p := range ps {
		if len(p.calls) != 1 || len(p.calls[0]) != 20 {
			t.Fatalf("provider %d: %d panggilan", i, len(p.calls))
		}
	}
}

func TestT03_ListProjects_NoStoreHeader(t *testing.T) {
	e := newHTTPEnv(t)
	w, _ := e.get(t, "/projects", e.s.A, "developer")
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("tanpa no-store")
	}
}

func TestT03_ListProjects_ReflectsMembershipChangeImmediately(t *testing.T) {
	e := newHTTPEnv(t)
	if _, err := e.s.d.ExecContext(context.Background(), `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, e.s.p["P1"], e.s.A); err != nil {
		t.Fatal(err)
	}
	_, m := e.get(t, "/projects", e.s.A, "developer")
	if itemNames(m)["P1"] != nil {
		t.Fatal("P1 masih muncul")
	}
}

var _ = db.TestDB
