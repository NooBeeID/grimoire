package deletegroup

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
	err      error
	pid, gid string
}

func (f *fakeExec) Execute(_ context.Context, pid, gid string, _ base.RequestMeta) error {
	f.pid, f.gid = pid, gid
	return f.err
}

func serve(f *fakeExec) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Delete("/projects/{projectID}/search/synonyms/{groupID}", (&handler{uc: f}).serve)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/projects/p1/search/synonyms/g1", nil))
	return w
}

func TestHandler204(t *testing.T) {
	f := &fakeExec{}
	w := serve(f)
	if w.Code != 204 || w.Body.Len() != 0 || f.pid != "p1" || f.gid != "g1" {
		t.Fatalf("%d %q %+v", w.Code, w.Body, f)
	}
}

func TestHandlerErrors(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{base.ErrGroupNotFound, 404},
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
