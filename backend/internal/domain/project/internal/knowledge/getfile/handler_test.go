package getfile

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

func serve(a *fakeAccess, f *fakeFiles, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/projects/{projectID}/knowledge/files", NewHandler(NewUsecase(a, f)).Handle)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandler_OK(t *testing.T) {
	ff := &fakeFiles{file: mkFile("# Login\n"), found: true}
	rec := serve(&fakeAccess{}, ff, "/projects/p1/knowledge/files?path=docs%2Flogin.md")
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") ||
		rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("code=%d hdr=%v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	for _, w := range []string{`"path":"docs/login.md"`, `"size_bytes":8`, `"version":"`, `"created_at":"2026-10-24T09:00:00Z"`,
		`"created_by":{`, `"updated_at":`, `"updated_by":{`, `"title":"Login"`} {
		if !strings.Contains(body, w) {
			t.Fatalf("missing %s in %s", w, body)
		}
	}
	if ff.path != "docs/login.md" {
		t.Fatalf("path=%q", ff.path)
	}
}

func TestHandler_UnicodePath(t *testing.T) {
	ff := &fakeFiles{file: mkFile("x"), found: true}
	serve(&fakeAccess{}, ff, "/projects/p1/knowledge/files?path=catatan%20rapat%2F%C3%A9.md")
	if ff.path != "catatan rapat/é.md" {
		t.Fatalf("path=%q", ff.path)
	}
}

func TestHandler_MissingPath(t *testing.T) {
	rec := serve(&fakeAccess{}, &fakeFiles{}, "/projects/p1/knowledge/files")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "path_empty") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		a    *fakeAccess
		f    *fakeFiles
		want int
	}{
		{"401", &fakeAccess{err: base.ErrUnauthenticated}, &fakeFiles{}, 401},
		{"404 project", &fakeAccess{err: base.ErrProjectNotFound}, &fakeFiles{}, 404},
		{"404 file", &fakeAccess{}, &fakeFiles{}, 404},
		{"500 bus", &fakeAccess{err: errors.New("secret-internal")}, &fakeFiles{}, 500},
		{"500 db", &fakeAccess{}, &fakeFiles{err: errors.New("secret-internal")}, 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serve(c.a, c.f, "/projects/p1/knowledge/files?path=a.md")
			if rec.Code != c.want {
				t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "secret-internal") {
				t.Fatal("bocor error internal")
			}
		})
	}
}
