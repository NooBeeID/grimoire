package movefile

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
	dto   base.FileDTO
	err   error
	calls int
	pid   string
	in    Request
	meta  base.RequestMeta
}

func (f *fakeExec) Execute(_ context.Context, pid string, in Request, meta base.RequestMeta) (base.FileDTO, error) {
	f.calls++
	f.pid, f.in, f.meta = pid, in, meta
	return f.dto, f.err
}

func serve(f *fakeExec, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/knowledge/files/move", NewHandler(f).Handle)
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/knowledge/files/move", strings.NewReader(body))
	req.Header.Set("User-Agent", "ua-test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler200(t *testing.T) {
	f := &fakeExec{dto: base.FileDTO{Path: "b.md", SizeBytes: 3, Version: "vv", UpdatedBy: base.UserRef{ID: "u", Label: "S"}}}
	w := serve(f, `{"from":"a.md","to":"b.md"}`)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body)
	}
	for _, k := range []string{`"path":"b.md"`, `"size_bytes":3`, `"version":"vv"`, `"updated_by":{"id":"u","label":"S"}`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("missing %s: %s", k, w.Body)
		}
	}
	if f.pid != "p1" || f.in.From != "a.md" || f.in.To != "b.md" || f.meta.UserAgent != "ua-test" {
		t.Fatalf("%+v", f)
	}
}

func TestHandlerBadBody(t *testing.T) {
	for _, b := range []string{`not json`, `{"from":"a","to":"b","x":1}`} {
		f := &fakeExec{}
		if w := serve(f, b); w.Code != 400 || f.calls != 0 {
			t.Fatalf("%q %d", b, w.Code)
		}
	}
}

func TestHandlerMissingFields(t *testing.T) {
	f := &fakeExec{err: base.Invalid(base.ReasonPathEmpty)}
	w := serve(f, `{}`)
	if w.Code != 400 || f.in.From != "" || f.in.To != "" || !strings.Contains(w.Body.String(), "path_empty") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandlerUnicodePassThrough(t *testing.T) {
	f := &fakeExec{}
	serve(f, `{"from":"catatan rapat/é.md","to":"arsip/é.md"}`)
	if f.in.From != "catatan rapat/é.md" || f.in.To != "arsip/é.md" {
		t.Fatalf("%+v", f.in)
	}
}

func TestHandlerErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{base.Invalid(base.ReasonSamePath), 400},
		{base.ErrUnauthenticated, 401},
		{base.ErrProjectNotFound, 404},
		{base.ErrFileNotFound, 404},
		{base.ErrPathExists, 409},
		{errors.New("secret db detail"), 500},
	}
	for _, c := range cases {
		w := serve(&fakeExec{err: c.err}, `{"from":"a.md","to":"b.md"}`)
		if w.Code != c.want {
			t.Fatalf("%v: %d", c.err, w.Code)
		}
		if c.want == 500 && strings.Contains(w.Body.String(), "secret") {
			t.Fatal("detail bocor")
		}
	}
}
