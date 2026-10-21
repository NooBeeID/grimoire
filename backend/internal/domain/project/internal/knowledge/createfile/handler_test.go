package createfile

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/common/errs"
)

type fakeExec struct {
	dto   base.FileDTO
	err   error
	calls int
	pid   string
	req   CreateRequest
}

func (f *fakeExec) Execute(_ context.Context, pid string, req CreateRequest, _ base.RequestMeta) (base.FileDTO, error) {
	f.calls++
	f.pid, f.req = pid, req
	return f.dto, f.err
}

func serve(f *fakeExec, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h := &Handler{uc: f}
	r.Post("/projects/{projectID}/knowledge/files", h.ServeHTTP)
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/knowledge/files", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler201(t *testing.T) {
	f := &fakeExec{dto: base.FileDTO{Path: "a.md", SizeBytes: 3, CreatedBy: base.UserRef{ID: "u", Label: "B"}}}
	w := serve(f, `{"path":"a.md","content":"abc"}`)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, k := range []string{`"size_bytes":3`, `"created_at"`, `"created_by":{"id":"u","label":"B"}`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("missing %s: %s", k, w.Body)
		}
	}
	if f.pid != "p1" || f.req.Path != "a.md" || f.req.Content != "abc" {
		t.Fatalf("%+v", f)
	}
}

func TestHandlerBadBody(t *testing.T) {
	for _, b := range []string{"", "{rusak", `{"path":"a.md","x":1}`} {
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
		{base.Invalid(base.ReasonHidden), 400, `"reason":"hidden"`},
		{base.TooLarge(base.ReasonTooLarge, 10), 413, `"max_bytes":10`},
		{base.ErrPathExists, 409, `"CONFLICT"`},
		{base.ErrProjectNotFound, 404, `"NOT_FOUND"`},
		{errors.New("rahasia-internal"), 500, `"INTERNAL"`},
	}
	for _, c := range cases {
		w := serve(&fakeExec{err: c.err}, `{"path":"a.md"}`)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.sub) {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body)
		}
		if c.code == 500 && strings.Contains(w.Body.String(), "rahasia-internal") {
			t.Error("bocor")
		}
	}
	_ = errs.CodeInvalid
}
