package getproject

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
)

type fakeExec struct {
	d   base.ProjectDetail
	err error
	got string
}

func (f *fakeExec) Execute(_ context.Context, id string) (base.ProjectDetail, error) {
	f.got = id
	return f.d, f.err
}

func serve(f *fakeExec, target, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/projects/{projectID}", newHandler(f).ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_Get_OK(t *testing.T) {
	role := "owner"
	f := &fakeExec{d: base.ProjectDetail{
		ProjectSummary: base.ProjectSummary{
			ProjectDTO: base.ProjectDTO{ID: "p1", Name: "P"}, MyRole: &role, AccessLevel: base.LevelOwner,
			IsMember: true, MemberCount: 2,
		},
		OwnerCount: 1,
	}}
	w := serve(f, "/projects/p1", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["owner_count"] != float64(1) || m["member_count"] != float64(2) || m["my_role"] != "owner" {
		t.Fatalf("%v", m)
	}
	c := m["counts"].(map[string]any)
	if v, ok := c["files"]; !ok || v != nil {
		t.Fatalf("counts %v", c)
	}
}

func TestHandler_Get_PassesRawParam(t *testing.T) {
	f := &fakeExec{}
	serve(f, "/projects/bukan-uuid", "")
	if f.got != "bukan-uuid" {
		t.Fatalf("got %q", f.got)
	}
}

func TestHandler_Get_NotFound404Body(t *testing.T) {
	w := serve(&fakeExec{err: base.ErrProjectNotFound}, "/projects/x", "")
	if w.Code != 404 {
		t.Fatalf("%d", w.Code)
	}
	var m struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details any    `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Error.Code != "NOT_FOUND" || m.Error.Message != "project tidak ditemukan" || m.Error.Details != nil {
		t.Fatalf("%s", w.Body.String())
	}
}

func TestHandler_Get_InternalIs500Generic(t *testing.T) {
	w := serve(&fakeExec{err: errors.New("pq: secret detail")}, "/projects/x", "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_Get_IgnoresQueryAndBody(t *testing.T) {
	f := &fakeExec{}
	w := serve(f, "/projects/p1?x=1", `{"a":1}`)
	if w.Code != 200 || f.got != "p1" {
		t.Fatalf("%d %q", w.Code, f.got)
	}
}
