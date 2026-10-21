package deletefolder

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
	out   Output
	err   error
	calls int
	p     params
	meta  base.RequestMeta
}

func (f *fakeExec) Execute(_ context.Context, p params, m base.RequestMeta) (Output, error) {
	f.calls++
	f.p, f.meta = p, m
	return f.out, f.err
}

func serve(f *fakeExec, url string, ua string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h := &handler{uc: f}
	r.Delete("/projects/{projectID}/knowledge/folders", h.serve)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler200(t *testing.T) {
	f := &fakeExec{out: Output{Deleted: 3}}
	w := serve(f, "/projects/p1/knowledge/folders?path=docs", "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"deleted":3}` || f.p.ProjectID != "p1" || f.p.Path != "docs" {
		t.Fatalf("%d %s %+v", w.Code, w.Body, f.p)
	}
}

func TestHandlerEncodedPath(t *testing.T) {
	f := &fakeExec{out: Output{Deleted: 1}}
	serve(f, "/projects/p1/knowledge/folders?path=docs%2Fauth", "")
	if f.p.Path != "docs/auth" {
		t.Fatalf("%q", f.p.Path)
	}
}

func TestHandlerMissingPath(t *testing.T) {
	f := &fakeExec{err: base.Invalid(base.ReasonPrefixRequired)}
	w := serve(f, "/projects/p1/knowledge/folders", "")
	if w.Code != 400 || f.p.Path != "" || !strings.Contains(w.Body.String(), "prefix_required") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandlerErrors(t *testing.T) {
	w := serve(&fakeExec{err: base.ErrProjectNotFound}, "/projects/nope/knowledge/folders?path=docs", "")
	if w.Code != 404 || !strings.Contains(w.Body.String(), "trace_id") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = serve(&fakeExec{err: base.ErrFolderNotFound}, "/projects/p1/knowledge/folders?path=docs", "")
	if w.Code != 404 || !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = serve(&fakeExec{err: base.Invalid(base.ReasonPathTraversal)}, "/projects/p1/knowledge/folders?path=../x", "")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "path_traversal") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = serve(&fakeExec{err: errors.New("secret detail")}, "/projects/p1/knowledge/folders?path=docs", "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret detail") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandlerMetaUserAgentTruncated(t *testing.T) {
	f := &fakeExec{out: Output{Deleted: 1}}
	serve(f, "/projects/p1/knowledge/folders?path=docs", strings.Repeat("a", 1000))
	if len(f.meta.UserAgent) != 512 {
		t.Fatalf("%d", len(f.meta.UserAgent))
	}
}
