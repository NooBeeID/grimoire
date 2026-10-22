package listkeys

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/lib/common/reqctx"
)

func serve(a *fakeAccess, r *fakeRepo, withID bool, target string) *httptest.ResponseRecorder {
	return serveAs(a, r, withID, "u1", "developer", target)
}

func serveAs(a *fakeAccess, r *fakeRepo, withID bool, id, role, target string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	router.Get("/projects/{projectID}/api-keys", newHandler(NewUsecase(a, r)).ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if withID {
		req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: id, Role: role, Email: "a@tim.id"}))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHandler_OK(t *testing.T) {
	r := &fakeRepo{keys: []base.Key{{ID: "1", Name: "ci", Prefix: "aaaaaaaa", CreatedAt: time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)}}}
	rec := serve(&fakeAccess{}, r, true, "/projects/p1/api-keys")
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("code=%d ct=%s", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, want := range []string{`"items":[`, `"status":"active"`, `"last_used_at":null`, `"created_at":"2026-10-23T10:00:00Z"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
}

func TestHandler_EmptyItemsIsArray(t *testing.T) {
	rec := serve(&fakeAccess{}, &fakeRepo{}, true, "/projects/p1/api-keys")
	if strings.TrimSpace(rec.Body.String()) != `{"items":[]}` {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestHandler_PassesProjectIDParam(t *testing.T) {
	r := &fakeRepo{}
	serve(&fakeAccess{}, r, true, "/projects/abc/api-keys?limit=1&x=y")
	if r.project != "abc" {
		t.Fatalf("project=%q", r.project)
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		a      *fakeAccess
		withID bool
		code   int
		want   string
	}{
		{"not-found", &fakeAccess{err: base.ErrProjectNotFound}, true, 404, `"code":"NOT_FOUND"`},
		{"unauthorized", &fakeAccess{}, false, 401, `"code":"UNAUTHORIZED"`},
		{"internal", &fakeAccess{err: errors.New("secret dsn leak")}, true, 500, `"code":"INTERNAL"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serve(c.a, &fakeRepo{}, c.withID, "/projects/p1/api-keys")
			body := rec.Body.String()
			if rec.Code != c.code || !strings.Contains(body, c.want) || !strings.Contains(body, "trace_id") || strings.Contains(body, "dsn leak") {
				t.Fatalf("code=%d body=%s", rec.Code, body)
			}
		})
	}
}

func TestHandler_PassesIdentityAsScope(t *testing.T) {
	r := &fakeRepo{}
	serveAs(&fakeAccess{}, r, true, "u1", "developer", "/projects/p1/api-keys")
	if r.scope != (base.OwnerScope{OwnerID: "u1"}) {
		t.Fatalf("developer scope=%+v", r.scope)
	}
	r = &fakeRepo{}
	serveAs(&fakeAccess{}, r, true, "s1", "super_admin", "/projects/p1/api-keys")
	if r.scope != (base.OwnerScope{All: true}) {
		t.Fatalf("super_admin scope=%+v", r.scope)
	}
}
