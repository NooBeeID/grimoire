package updategroup

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
	dto      base.GroupDTO
	err      error
	calls    int
	pid, gid string
	in       Input
}

func (f *fakeExec) Execute(_ context.Context, pid, gid string, in Input, _ base.RequestMeta) (base.GroupDTO, error) {
	f.calls++
	f.pid, f.gid, f.in = pid, gid, in
	return f.dto, f.err
}

func serve(f *fakeExec, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Put("/projects/{projectID}/search/synonyms/{groupID}", (&handler{uc: f}).serve)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/projects/p1/search/synonyms/g1", strings.NewReader(body)))
	return w
}

func TestHandler200(t *testing.T) {
	f := &fakeExec{dto: base.GroupDTO{ID: "g1", Terms: []string{"a", "b"}, UpdatedByLabel: "Sari"}}
	w := serve(f, `{"terms":["a","b"]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"updated_by_label":"Sari"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if f.pid != "p1" || f.gid != "g1" || len(f.in.Terms) != 2 {
		t.Fatalf("%+v", f)
	}
}

func TestHandlerBadBody(t *testing.T) {
	for _, b := range []string{"", "{rusak", `{"terms":"a"}`, `{"terms":["a","b"],"x":1}`} {
		f := &fakeExec{}
		if w := serve(f, b); w.Code != 400 || f.calls != 0 {
			t.Errorf("%q: %d", b, w.Code)
		}
	}
}

func TestHandlerErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
		sub  string
	}{
		{base.Invalid(base.ReasonInvalidGroupID), 400, `"reason":"invalid_group_id"`},
		{base.TermInUse([]string{"x"}), 409, `"reason":"term_in_use"`},
		{base.ErrGroupNotFound, 404, `"NOT_FOUND"`},
		{base.ErrProjectNotFound, 404, `"NOT_FOUND"`},
		{errors.New("rahasia-internal"), 500, `"INTERNAL"`},
	}
	for _, c := range cases {
		w := serve(&fakeExec{err: c.err}, `{"terms":["a","b"]}`)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.sub) || strings.Contains(w.Body.String(), "rahasia-internal") {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body)
		}
	}
}
