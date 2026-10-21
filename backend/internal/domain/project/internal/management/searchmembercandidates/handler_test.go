package searchmembercandidates

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
	items []base.CandidateDTO
	err   error
	got   Input
}

func (f *fakeExec) Execute(_ context.Context, in Input) ([]base.CandidateDTO, error) {
	f.got = in
	return f.items, f.err
}

func serve(f *fakeExec, target, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/projects/{projectID}/member-candidates", newHandler(f).ServeHTTP)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, strings.NewReader(body)))
	return w
}

func TestHandler_Get_OK(t *testing.T) {
	f := &fakeExec{items: []base.CandidateDTO{{ID: "c", Name: "Budiman", Email: "b@x.id"}}}
	w := serve(f, "/projects/p1/member-candidates?q=bud", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	var m struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || len(m.Items) != 1 ||
		m.Items[0]["id"] != "c" || m.Items[0]["name"] != "Budiman" || m.Items[0]["email"] != "b@x.id" {
		t.Fatalf("%s %v", w.Body.String(), err)
	}
}

func TestHandler_Get_EmptyItemsIsArray(t *testing.T) {
	w := serve(&fakeExec{items: []base.CandidateDTO{}}, "/projects/p1/member-candidates", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_Get_PassesRawParams(t *testing.T) {
	f := &fakeExec{}
	serve(f, "/projects/bukan-uuid/member-candidates?q=%20bud%20&limit=abc", "")
	if f.got.ProjectID != "bukan-uuid" || f.got.RawQ != " bud " || f.got.RawLimit != "abc" {
		t.Fatalf("%+v", f.got)
	}
}

func errBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	e, _ := m["error"].(map[string]any)
	return e
}

func TestHandler_Get_BadLimit400Body(t *testing.T) {
	_, err := parseQuery("a", "0")
	w := serve(&fakeExec{err: err}, "/projects/p1/member-candidates?q=a&limit=0", "")
	e := errBody(t, w)
	if w.Code != 400 || e["code"] != "INVALID_ARGUMENT" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	f := e["details"].(map[string]any)["fields"].(map[string]any)
	if f["limit"] != "limit harus 1-50" {
		t.Fatalf("%v", f)
	}
}

func TestHandler_Get_Forbidden403Body(t *testing.T) {
	w := serve(&fakeExec{err: base.ErrForbidden}, "/projects/p1/member-candidates?q=a", "")
	e := errBody(t, w)
	if w.Code != 403 || e["code"] != "FORBIDDEN" || e["message"] != "akses ditolak" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_Get_NotFound404Body(t *testing.T) {
	w := serve(&fakeExec{err: base.ErrProjectNotFound}, "/projects/p1/member-candidates?q=a", "")
	e := errBody(t, w)
	if w.Code != 404 || e["code"] != "NOT_FOUND" || e["message"] != "project tidak ditemukan" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_Get_InternalIs500Generic(t *testing.T) {
	w := serve(&fakeExec{err: errors.New("secret db detail")}, "/projects/p1/member-candidates?q=a", "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_Get_IgnoresBody(t *testing.T) {
	f := &fakeExec{items: []base.CandidateDTO{}}
	w := serve(f, "/projects/p1/member-candidates?q=a", "{bukan json")
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
}
