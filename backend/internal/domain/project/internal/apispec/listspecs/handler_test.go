package listspecs

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/reqctx"
)

func serve(g *fakeGuard, l *fakeLister, withID bool, target string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	router.Get("/projects/{projectID}/api-specs", newHandler(NewUsecase(g, l, 100)).ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if withID {
		req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "u1", Role: "developer", Email: "a@tim.id"}))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func sample(name string, sp *string) base.SpecSummary {
	ts := time.Date(2026, 10, 23, 9, 0, 0, 0, time.FixedZone("x", 7*3600))
	return base.SpecSummary{Name: name, Title: "T", OpenAPIVersion: "3.1.0", APIVersion: "1", Format: "yaml",
		SourcePath: sp, SizeBytes: 10, EndpointCount: 2, CreatedAt: ts, CreatedByLabel: "Budi", UpdatedAt: ts, UpdatedByLabel: "Budi"}
}

func TestHandler_OK(t *testing.T) {
	l := &fakeLister{items: []base.SpecSummary{sample("auth", nil), sample("payment", nil)}}
	rec := serve(&fakeGuard{}, l, true, "/projects/p1/api-specs")
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("code=%d hdr=%v", rec.Code, rec.Header())
	}
	var body struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Limit int              `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 2 || body.Limit != 100 || body.Items[0]["name"] != "auth" || body.Items[1]["name"] != "payment" {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if len(body.Items[0]) != 12 {
		t.Fatalf("keys=%v", body.Items[0])
	}
	for _, k := range []string{"content", "content_hash"} {
		if _, ok := body.Items[0][k]; ok {
			t.Fatalf("leaks %s", k)
		}
	}
	if body.Items[0]["source_path"] != nil || body.Items[0]["created_at"] != "2026-10-23T02:00:00Z" {
		t.Fatalf("item=%v", body.Items[0])
	}
}

func TestHandler_EmptyItemsIsArray(t *testing.T) {
	rec := serve(&fakeGuard{}, &fakeLister{}, true, "/projects/p1/api-specs")
	if !strings.Contains(rec.Body.String(), `"items":[]`) || !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestHandler_PassesProjectIDParam(t *testing.T) {
	g := &fakeGuard{}
	serve(g, &fakeLister{}, true, "/projects/abc/api-specs")
	if g.project != "abc" {
		t.Fatalf("project=%q", g.project)
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		g      *fakeGuard
		l      *fakeLister
		withID bool
		code   int
		want   string
	}{
		{"not-found", &fakeGuard{err: base.ErrProjectNotFound}, &fakeLister{}, true, 404, `"message":"project tidak ditemukan"`},
		{"unauthorized", &fakeGuard{}, &fakeLister{}, false, 401, `"code":"UNAUTHORIZED"`},
		{"guard-internal", &fakeGuard{err: errors.New("secret dsn leak")}, &fakeLister{}, true, 500, `"code":"INTERNAL"`},
		{"store-internal", &fakeGuard{}, &fakeLister{err: errors.New("secret dsn leak")}, true, 500, `"code":"INTERNAL"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serve(c.g, c.l, c.withID, "/projects/p1/api-specs")
			body := rec.Body.String()
			if rec.Code != c.code || !strings.Contains(body, c.want) || !strings.Contains(body, "trace_id") || strings.Contains(body, "dsn leak") {
				t.Fatalf("code=%d body=%s", rec.Code, body)
			}
		})
	}
}
