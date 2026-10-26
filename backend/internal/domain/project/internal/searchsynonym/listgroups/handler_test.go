package listgroups

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
)

type fakeExec struct {
	items []base.GroupDTO
	err   error
	pid   string
}

func (f *fakeExec) Execute(_ context.Context, pid string) ([]base.GroupDTO, error) {
	f.pid = pid
	return f.items, f.err
}

func serve(f *fakeExec) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/projects/{projectID}/search/synonyms", (&handler{uc: f}).serve)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/p1/search/synonyms", nil))
	return w
}

func TestHandler_EmptyIsArray(t *testing.T) {
	for _, items := range [][]base.GroupDTO{nil, {}} {
		w := serve(&fakeExec{items: items})
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"items":[]}` {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
}

func TestHandler_Items(t *testing.T) {
	f := &fakeExec{items: []base.GroupDTO{{ID: "g1", Terms: []string{"a", "b"}}}}
	w := serve(f)
	if w.Code != 200 || f.pid != "p1" || !strings.Contains(w.Body.String(), `"terms":["a","b"]`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandler_Errors(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{base.ErrProjectNotFound, 404},
		{base.ErrUnauthenticated, 401},
		{errors.New("rahasia-internal"), 500},
	}
	for _, c := range cases {
		w := serve(&fakeExec{err: c.err})
		if w.Code != c.code || strings.Contains(w.Body.String(), "rahasia-internal") {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body)
		}
	}
}
