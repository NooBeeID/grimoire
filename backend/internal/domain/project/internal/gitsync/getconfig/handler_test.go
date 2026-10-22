package getconfig

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/gitsync/base"
)

func serve(a *fakeAccess, r *fakeRepo, c *fakeCred, target string) *httptest.ResponseRecorder {
	uc, _ := newUC(a, r, c, true)
	router := chi.NewRouter()
	router.Get("/projects/{projectID}/git-sync", newHandler(uc).ServeHTTP)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandler_OK(t *testing.T) {
	rec := serve(&fakeAccess{}, &fakeRepo{cfg: sampleCfg()}, &fakeCred{st: CredentialStatus{Found: true}}, "/projects/p1/git-sync?kind=knowledge")
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	for _, want := range []string{`"kind":"knowledge"`, `"has_credential":true`, `"feature_enabled":true`, `"credential_unreadable":false`,
		`"last_synced_at":"2026-10-22T14:05:00Z"`, `"last_synced_by":{"id":"u-other","label":"Rina"}`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
	if strings.Contains(strings.ReplaceAll(body, "has_credential", ""), "token") {
		t.Errorf("kata token: %s", body)
	}
}

func TestHandler_NullConfig(t *testing.T) {
	rec := serve(&fakeAccess{}, &fakeRepo{}, &fakeCred{}, "/projects/p1/git-sync?kind=knowledge")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"config":null`) || !strings.Contains(rec.Body.String(), `"has_credential":false`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestHandler_PassesParams(t *testing.T) {
	r := &fakeRepo{}
	serve(&fakeAccess{}, r, &fakeCred{}, "/projects/abc/git-sync?kind=api_spec")
	if r.kind != base.KindAPISpec || r.proj != "abc" {
		t.Fatalf("%+v", r)
	}
}

func TestHandler_Errors(t *testing.T) {
	for name, tc := range map[string]struct {
		a      *fakeAccess
		r      *fakeRepo
		target string
		code   int
		body   string
	}{
		"no kind":    {&fakeAccess{}, &fakeRepo{}, "/projects/p/git-sync", 400, "INVALID_ARGUMENT"},
		"bad kind":   {&fakeAccess{}, &fakeRepo{}, "/projects/p/git-sync?kind=foo", 400, `"kind"`},
		"non-member": {&fakeAccess{err: base.ErrProjectNotFound}, &fakeRepo{}, "/projects/p/git-sync?kind=knowledge", 404, "NOT_FOUND"},
		"unauth":     {&fakeAccess{err: base.ErrUnauthenticated}, &fakeRepo{}, "/projects/p/git-sync?kind=knowledge", 401, "UNAUTHORIZED"},
		"repo error": {&fakeAccess{}, &fakeRepo{err: errors.New("pg secret detail")}, "/projects/p/git-sync?kind=knowledge", 500, ""},
	} {
		rec := serve(tc.a, tc.r, &fakeCred{}, tc.target)
		if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.body) || strings.Contains(rec.Body.String(), "secret detail") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}

func TestHandler_InProgress(t *testing.T) {
	rec := serve(&fakeAccess{}, &fakeRepo{cfg: sampleCfg(), inProgress: true}, &fakeCred{}, "/projects/p1/git-sync?kind=knowledge")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"in_progress":true`) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = serve(&fakeAccess{}, &fakeRepo{inProgress: true}, &fakeCred{}, "/projects/p1/git-sync?kind=knowledge")
	if !strings.Contains(rec.Body.String(), `"config":null`) || !strings.Contains(rec.Body.String(), `"in_progress":true`) {
		t.Fatalf("%s", rec.Body)
	}
	rec = serve(&fakeAccess{}, &fakeRepo{cfg: sampleCfg()}, &fakeCred{}, "/projects/p1/git-sync?kind=knowledge")
	if !strings.Contains(rec.Body.String(), `"in_progress":false`) {
		t.Fatalf("%s", rec.Body)
	}
	rec = serve(&fakeAccess{}, &fakeRepo{inProgressErr: errors.New("x")}, &fakeCred{}, "/projects/p1/git-sync?kind=knowledge")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"in_progress":false`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = serve(&fakeAccess{err: base.ErrProjectNotFound}, &fakeRepo{}, &fakeCred{}, "/projects/p1/git-sync?kind=knowledge")
	if rec.Code != 404 || strings.Contains(rec.Body.String(), "in_progress") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
