package updatefile

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
	in    Input
}

func (f *fakeExec) Execute(_ context.Context, in Input) (base.FileDTO, error) {
	f.calls++
	f.in = in
	return f.dto, f.err
}

func serve(f *fakeExec, target, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Put("/projects/{projectID}/knowledge/files", (&Handler{uc: f}).ServeHTTP)
	req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(body))
	req.Header.Set("User-Agent", "ua-test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const target = "/projects/p1/knowledge/files?path=docs%2Fauth%2Flogin.md"

func TestHandler200(t *testing.T) {
	f := &fakeExec{dto: base.FileDTO{Path: "docs/auth/login.md", SizeBytes: 3, Version: "v", UpdatedBy: base.UserRef{ID: "u", Label: "S"}}}
	w := serve(f, target, `{"content":"abc","version":"vvv"}`)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, k := range []string{`"size_bytes":3`, `"updated_by":{"id":"u","label":"S"}`, `"version":"v"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("missing %s: %s", k, w.Body)
		}
	}
	if f.in.ProjectID != "p1" || f.in.Path != "docs/auth/login.md" || f.in.Content == nil || *f.in.Content != "abc" ||
		f.in.Version != "vvv" || f.in.Meta.UserAgent != "ua-test" {
		t.Fatalf("%+v", f.in)
	}
}

func TestHandlerContentOmitted(t *testing.T) {
	f := &fakeExec{}
	serve(f, target, `{"version":"v"}`)
	if f.in.Content != nil {
		t.Fatal("content harus nil")
	}
	serve(f, target, `{"content":"","version":"v"}`)
	if f.in.Content == nil || *f.in.Content != "" {
		t.Fatal("content kosong harus non-nil")
	}
}

func TestHandlerBadBody(t *testing.T) {
	for _, b := range []string{"", "{rusak", `{"content":"x","version":"v","extra":1}`} {
		f := &fakeExec{}
		if w := serve(f, target, b); w.Code != 400 || f.calls != 0 {
			t.Errorf("%q: %d", b, w.Code)
		}
	}
}

func TestHandlerErrorMapping(t *testing.T) {
	cur := base.ConflictCurrent{Content: "isi", Version: "vv", UpdatedByLabel: "Sari"}
	cases := []struct {
		err  error
		code int
		subs []string
	}{
		{base.VersionConflict(cur), 409, []string{`"CONFLICT"`, `"reason":"version_conflict"`, `"current":{"content":"isi","version":"vv"`, `"updated_by_label":"Sari"`}},
		{base.Invalid(base.ReasonInvalidVersion), 400, []string{`"reason":"invalid_version"`}},
		{base.TooLarge(base.ReasonTooLarge, 10), 413, []string{`"max_bytes":10`}},
		{base.ErrFileNotFound, 404, []string{`"NOT_FOUND"`}},
		{base.ErrProjectNotFound, 404, []string{`"NOT_FOUND"`}},
		{base.ErrUnauthenticated, 401, []string{`"UNAUTHORIZED"`}},
		{errors.New("rahasia-internal"), 500, []string{`"INTERNAL"`}},
	}
	for _, c := range cases {
		w := serve(&fakeExec{err: c.err}, target, `{"content":"x","version":"v"}`)
		if w.Code != c.code {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body)
		}
		for _, s := range c.subs {
			if !strings.Contains(w.Body.String(), s) {
				t.Errorf("%v: missing %s in %s", c.err, s, w.Body)
			}
		}
		if c.code == 500 && strings.Contains(w.Body.String(), "rahasia-internal") {
			t.Error("bocor")
		}
	}
}
