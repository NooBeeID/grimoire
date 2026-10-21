package deletefile

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

type fakeExec struct {
	err   error
	calls int
	p     params
}

func (f *fakeExec) Execute(_ context.Context, p params, _ base.RequestMeta) error {
	f.calls++
	f.p = p
	return f.err
}

func serve(f *fakeExec, url string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h := &handler{uc: f}
	r.Delete("/projects/{projectID}/knowledge/files", h.serve)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, url, nil))
	return w
}

func TestHandler204(t *testing.T) {
	f := &fakeExec{}
	w := serve(f, "/projects/p1/knowledge/files?path=docs%2Fa.md")
	if w.Code != 204 || w.Body.Len() != 0 || f.p.ProjectID != "p1" || f.p.Path != "docs/a.md" {
		t.Fatalf("%d %q %+v", w.Code, w.Body, f.p)
	}
}

func TestHandlerMissingPath(t *testing.T) {
	f := &fakeExec{err: base.Invalid(base.ReasonPathEmpty)}
	w := serve(f, "/projects/p1/knowledge/files")
	if w.Code != 400 || f.p.Path != "" || !strings.Contains(w.Body.String(), "path_empty") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandlerErrors(t *testing.T) {
	w := serve(&fakeExec{err: base.ErrFileNotFound}, "/projects/p1/knowledge/files?path=a.md")
	if w.Code != 404 || !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = serve(&fakeExec{err: base.Invalid(base.ReasonPathTraversal)}, "/projects/p1/knowledge/files?path=../x")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "path_traversal") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = serve(&fakeExec{err: errors.New("secret detail")}, "/projects/p1/knowledge/files?path=a.md")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret detail") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
