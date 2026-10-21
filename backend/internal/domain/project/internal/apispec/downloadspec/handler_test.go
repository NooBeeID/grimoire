package downloadspec

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/reqctx"
)

func serve(f *fakeFinder, g *fakeGuard, withID bool) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	serveTo(rec, f, g, withID)
	return rec
}

func serveTo(w http.ResponseWriter, f *fakeFinder, g *fakeGuard, withID bool) {
	router := chi.NewRouter()
	router.Get("/projects/{projectID}/api-specs/{name}/raw", newHandler(NewUsecase(f, g), nil).ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, "/projects/p1/api-specs/payment/raw", nil)
	if withID {
		req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "u1", Role: "developer", Email: "a@tim.id"}))
	}
	router.ServeHTTP(w, req)
}

func TestHandler_YAMLHeadersAndBody(t *testing.T) {
	content := "openapi: 3.0.3\r\n<script>alert(1)</script>"
	rec := serve(&fakeFinder{found: true, spec: spec(content, "yaml")}, &fakeGuard{}, true)
	h := rec.Header()
	if rec.Code != 200 || h.Get("Content-Type") != "application/yaml; charset=utf-8" ||
		h.Get("Content-Disposition") != `attachment; filename="payment.yaml"` ||
		h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "no-store" ||
		h.Get("Content-Length") != strconv.Itoa(len(content)) || rec.Body.String() != content {
		t.Fatalf("code=%d hdr=%v body=%q", rec.Code, h, rec.Body.String())
	}
}

func TestHandler_JSON(t *testing.T) {
	rec := serve(&fakeFinder{found: true, spec: spec(`{"a":1}`, "json")}, &fakeGuard{}, true)
	if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" ||
		rec.Header().Get("Content-Disposition") != `attachment; filename="payment.json"` {
		t.Fatalf("hdr=%v", rec.Header())
	}
}

func TestHandler_LargeBody(t *testing.T) {
	content := strings.Repeat("0123456789abcdef", 5<<16) // 5 MiB
	rec := serve(&fakeFinder{found: true, spec: spec(content, "yaml")}, &fakeGuard{}, true)
	if sha256.Sum256(rec.Body.Bytes()) != sha256.Sum256([]byte(content)) {
		t.Fatal("hash beda")
	}
}

func TestHandler_Errors(t *testing.T) {
	cases := []struct {
		name string
		f    *fakeFinder
		g    *fakeGuard
		id   bool
		code int
		msg  string
	}{
		{"spec 404", &fakeFinder{}, &fakeGuard{}, true, 404, "spec tidak ditemukan"},
		{"project 404", &fakeFinder{found: true}, &fakeGuard{err: base.ErrProjectNotFound}, true, 404, "project tidak ditemukan"},
		{"401", &fakeFinder{}, &fakeGuard{}, false, 401, "perlu login"},
		{"500", &fakeFinder{err: errors.New("secret db detail")}, &fakeGuard{}, true, 500, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serve(c.f, c.g, c.id)
			if rec.Code != c.code || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") ||
				rec.Header().Get("Content-Disposition") != "" || !strings.Contains(rec.Body.String(), c.msg) ||
				strings.Contains(rec.Body.String(), "secret db detail") {
				t.Fatalf("code=%d hdr=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
			}
		})
	}
}

type failWriter struct {
	hdr  http.Header
	code int
}

func (w *failWriter) Header() http.Header       { return w.hdr }
func (w *failWriter) WriteHeader(c int)         { w.code = c }
func (w *failWriter) Write([]byte) (int, error) { return 0, errors.New("client gone") }

func TestHandler_WriteFailureNoPanic(t *testing.T) {
	w := &failWriter{hdr: http.Header{}}
	serveTo(w, &fakeFinder{found: true, spec: spec("a: 1", "yaml")}, &fakeGuard{}, true)
	if w.code != 200 {
		t.Fatalf("code=%d", w.code)
	}
}
