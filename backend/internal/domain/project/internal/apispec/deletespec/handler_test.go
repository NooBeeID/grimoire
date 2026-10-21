package deletespec

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/reqctx"
)

func serve(r *fakeRepo, g *fakeGuard, withID bool, body string) (*httptest.ResponseRecorder, *fakeAudit) {
	a := &fakeAudit{repo: r}
	router := chi.NewRouter()
	router.Delete("/projects/{projectID}/api-specs/{name}", newHandler(NewUsecase(r, g, a)).ServeHTTP)
	req := httptest.NewRequest(http.MethodDelete, "/projects/"+pid+"/api-specs/payment", strings.NewReader(body))
	req.Header.Set("User-Agent", strings.Repeat("a", 600))
	if withID {
		req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "u1", Name: "Budi"}))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec, a
}

func TestHandler_NoContent(t *testing.T) {
	rec, a := serve(&fakeRepo{found: true, count: 3}, &fakeGuard{}, true, `{"ignored":true}`)
	if rec.Code != 204 || rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "" {
		t.Fatalf("code=%d body=%q hdr=%v", rec.Code, rec.Body.String(), rec.Header())
	}
	if len(a.events) != 1 || a.events[0].Target != "payment" || len(a.events[0].Meta.UserAgent) != 512 {
		t.Fatalf("events=%+v", a.events)
	}
}

func TestHandler_Errors(t *testing.T) {
	cases := []struct {
		name   string
		r      *fakeRepo
		g      *fakeGuard
		withID bool
		code   int
		msg    string
	}{
		{"spec", &fakeRepo{found: false}, &fakeGuard{}, true, 404, "spec tidak ditemukan"},
		{"project", &fakeRepo{found: true}, &fakeGuard{err: base.ErrProjectNotFound}, true, 404, "project tidak ditemukan"},
		{"unauth", &fakeRepo{found: true}, &fakeGuard{}, false, 401, "perlu login"},
	}
	for _, c := range cases {
		rec, a := serve(c.r, c.g, c.withID, "")
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.msg) || len(a.events) != 0 {
			t.Fatalf("%s: code=%d body=%s", c.name, rec.Code, rec.Body.String())
		}
	}
}

func TestHandler_InternalNoLeak(t *testing.T) {
	rec, _ := serve(&fakeRepo{found: true, lockErr: errString("secret-detail")}, &fakeGuard{}, true, "")
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "secret-detail") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

type errString string

func (e errString) Error() string { return string(e) }
