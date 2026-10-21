package uploadfiles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

func newRouter(access fakeAccess) (*chi.Mux, *fakeRepo) {
	log := &[]string{}
	r := &fakeRepo{log: log, existing: map[string][]byte{}}
	uc := NewUsecase(r, access, &fakeAudit{log: log}, base.Limits{MaxFileBytes: 100, MaxUploadBytes: 10000, MaxUploadFiles: 10, MaxProjectFiles: 100})
	h := &Handler{uc: uc, limits: uc.limits}
	rt := chi.NewRouter()
	rt.Post("/projects/{projectID}/knowledge/upload", h.Upload)
	return rt, r
}

func TestHandler200(t *testing.T) {
	rt, repo := newRouter(fakeAccess{})
	b, ct := buildBody(t, []tp{
		{field: "files", name: "docs/auth/login.md", data: "# Login"},
		{field: "files", name: ".git/config", data: "x"},
	})
	rq := httptest.NewRequest(http.MethodPost, "/projects/p1/knowledge/upload", b)
	rq.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, rq)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, k := range []string{`"created":1`, `"skipped":1`, `"path":"docs/auth/login.md"`, `"reason":"hidden"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("missing %s: %s", k, w.Body)
		}
	}
	if len(repo.inserted) != 1 || repo.inserted[0].Path != "docs/auth/login.md" {
		t.Fatalf("%+v", repo.inserted)
	}
}

func TestHandlerNotMember404BodyNotRead(t *testing.T) {
	rt, repo := newRouter(fakeAccess{err: base.ErrProjectNotFound})
	called := false
	rq := httptest.NewRequest(http.MethodPost, "/projects/p1/knowledge/upload", &spyReader{called: &called})
	rq.Header.Set("Content-Type", "multipart/form-data; boundary=b")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, rq)
	if w.Code != 404 || called || repo.inTxN != 0 {
		t.Fatalf("%d called=%v", w.Code, called)
	}
}

type spyReader struct{ called *bool }

func (s *spyReader) Read([]byte) (int, error) { *s.called = true; return 0, context.Canceled }

func TestHandlerNotMultipart400(t *testing.T) {
	rt, _ := newRouter(fakeAccess{})
	rq := httptest.NewRequest(http.MethodPost, "/projects/p1/knowledge/upload", strings.NewReader(`{}`))
	rq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, rq)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "not_multipart") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandlerNoFiles400(t *testing.T) {
	rt, _ := newRouter(fakeAccess{})
	b, ct := buildBody(t, []tp{{field: "meta", name: "a.md", data: "x"}})
	rq := httptest.NewRequest(http.MethodPost, "/projects/p1/knowledge/upload", b)
	rq.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, rq)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "no_files") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandler413AndMethod(t *testing.T) {
	rt, repo := newRouter(fakeAccess{})
	b, ct := buildBody(t, []tp{{field: "files", name: "a.md", data: strings.Repeat("a", 90)}})
	rt2 := chi.NewRouter()
	uc := NewUsecase(repo, fakeAccess{}, &fakeAudit{log: repo.log}, base.Limits{MaxFileBytes: 100, MaxUploadBytes: 50, MaxUploadFiles: 10, MaxProjectFiles: 10})
	h := &Handler{uc: uc, limits: uc.limits}
	rt2.Post("/projects/{projectID}/knowledge/upload", h.Upload)
	rq := httptest.NewRequest(http.MethodPost, "/projects/p1/knowledge/upload", b)
	rq.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	rt2.ServeHTTP(w, rq)
	if w.Code != 413 || !strings.Contains(w.Body.String(), "upload_too_large") || repo.inTxN != 0 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	rt.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/p1/knowledge/upload", nil))
	if w.Code != 405 {
		t.Fatalf("%d", w.Code)
	}
}
