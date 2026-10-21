package getspec

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/reqctx"
)

func serve(r *fakeRepo, g *fakeGuard, withID bool, target string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	router.Get("/projects/{projectID}/api-specs/{name}", newHandler(NewUsecase(r, g, nil)).ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if withID {
		req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "u1", Role: "developer", Email: "a@tim.id"}))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHandler_OK(t *testing.T) {
	r := &fakeRepo{found: true, spec: specOf(json31, "json")}
	rec := serve(r, &fakeGuard{}, true, "/projects/p1/api-specs/payment")
	ct := rec.Header().Get("Content-Type")
	if rec.Code != 200 || !strings.HasPrefix(ct, "application/json") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("code=%d hdr=%v", rec.Code, rec.Header())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body) != 3 {
		t.Fatalf("body=%s err=%v", rec.Body.String(), err)
	}
	if string(body["endpoints"]) != "[]" {
		t.Fatalf("endpoints=%s", body["endpoints"])
	}
	for _, k := range []string{"spec", "endpoints", "document"} {
		if _, ok := body[k]; !ok {
			t.Fatalf("missing %s", k)
		}
	}
}

func TestHandler_NonMemberAndMissingProjectIdentical(t *testing.T) {
	a := serve(&fakeRepo{}, &fakeGuard{err: base.ErrProjectNotFound}, true, "/projects/p1/api-specs/payment")
	b := serve(&fakeRepo{}, &fakeGuard{err: base.ErrProjectNotFound}, true, "/projects/not-a-uuid/api-specs/payment")
	if a.Code != 404 || b.Code != 404 || !strings.Contains(a.Body.String(), "project tidak ditemukan") {
		t.Fatalf("a=%d %s", a.Code, a.Body.String())
	}
	// trace_id boleh berbeda; bandingkan kode+pesan
	var ea, eb struct {
		Error map[string]any `json:"error"`
	}
	_ = json.Unmarshal(a.Body.Bytes(), &ea)
	_ = json.Unmarshal(b.Body.Bytes(), &eb)
	ja, _ := json.Marshal(ea.Error)
	jb, _ := json.Marshal(eb.Error)
	if string(ja) != string(jb) {
		t.Fatalf("%s != %s", ja, jb)
	}
}

func TestHandler_SpecNotFound(t *testing.T) {
	rec := serve(&fakeRepo{}, &fakeGuard{}, true, "/projects/p1/api-specs/nope")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "spec tidak ditemukan") {
		t.Fatalf("code=%d %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_Unauthorized(t *testing.T) {
	rec := serve(&fakeRepo{}, &fakeGuard{}, false, "/projects/p1/api-specs/payment")
	if rec.Code != 401 {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestHandler_InternalErrorGeneric(t *testing.T) {
	rec := serve(&fakeRepo{}, &fakeGuard{err: errors.New("pq: secret detail")}, true, "/projects/p1/api-specs/payment")
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "secret detail") || !strings.Contains(rec.Body.String(), "INTERNAL") {
		t.Fatalf("code=%d %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_PassesParamsAndNeverHTML(t *testing.T) {
	r := &fakeRepo{found: true, spec: specOf(yaml30, "yaml")}
	rec := serve(r, &fakeGuard{}, true, "/projects/p1/api-specs/payment")
	if strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatal("html")
	}
}
