package creategroup

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
	dto   base.GroupDTO
	err   error
	calls int
	pid   string
	in    Input
}

func (f *fakeExec) Execute(_ context.Context, pid string, in Input, _ base.RequestMeta) (base.GroupDTO, error) {
	f.calls++
	f.pid, f.in = pid, in
	return f.dto, f.err
}

func serve(f *fakeExec, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/search/synonyms", (&handler{uc: f}).serve)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/projects/p1/search/synonyms", strings.NewReader(body)))
	return w
}

func TestHandler201(t *testing.T) {
	f := &fakeExec{dto: base.GroupDTO{ID: "g1", Terms: []string{"login", "masuk"}, CreatedByLabel: "Budi"}}
	w := serve(f, `{"terms":["login","masuk"]}`)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, k := range []string{`"id":"g1"`, `"terms":["login","masuk"]`, `"created_by_label":"Budi"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("missing %s: %s", k, w.Body)
		}
	}
	if f.pid != "p1" || len(f.in.Terms) != 2 {
		t.Fatalf("%+v", f)
	}
}

func TestHandlerBadBody(t *testing.T) {
	for _, b := range []string{"", "{rusak", `{"terms":"login"}`, `{"terms":[1,2]}`, `{"terms":["a","b"],"x":1}`} {
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
		{base.Invalid(base.ReasonTooFewTerms), 400, `"reason":"too_few_terms"`},
		{base.TermInUse([]string{"login"}), 409, `"terms":["login"]`},
		{base.GroupLimit(), 409, `"reason":"group_limit"`},
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
