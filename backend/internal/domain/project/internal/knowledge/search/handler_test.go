package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

type fakeExec struct {
	res base.SearchResultDTO
	err error
	got Input
}

func (f *fakeExec) Execute(_ context.Context, in Input) (base.SearchResultDTO, error) {
	f.got = in
	return f.res, f.err
}

func serve(f *fakeExec, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/projects/{projectID}/knowledge/search", newHandler(f).ServeHTTP)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func TestHandler_PassesParams(t *testing.T) {
	f := &fakeExec{res: base.SearchResultDTO{Items: []base.SearchItemDTO{{Path: "a.md", Snippet: []base.Segment{{Text: "x", Highlight: true}}}}, Total: 1}}
	w := serve(f, "/projects/p1/knowledge/search?q=login&prefix=docs&limit=5")
	if w.Code != 200 || f.got != (Input{ProjectID: "p1", Query: "login", Prefix: "docs", Limit: "5"}) {
		t.Fatalf("%d %+v", w.Code, f.got)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cache-control")
	}
	var m struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || m.Total != 1 || len(m.Items) != 1 {
		t.Fatalf("%v %s", err, w.Body.String())
	}
}

func TestHandler_NoQ(t *testing.T) {
	f := &fakeExec{res: base.SearchResultDTO{Items: []base.SearchItemDTO{}}}
	w := serve(f, "/projects/p1/knowledge/search")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"items":[],"total":0,"expanded":false}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_EncodedQuery(t *testing.T) {
	f := &fakeExec{}
	serve(f, "/projects/p1/knowledge/search?q=%22reset%20password%22")
	if f.got.Query != `"reset password"` {
		t.Fatalf("%q", f.got.Query)
	}
}

func TestHandler_SnippetNoHTMLWrapping(t *testing.T) {
	f := &fakeExec{res: base.SearchResultDTO{Items: []base.SearchItemDTO{{Path: "a.md", Snippet: []base.Segment{{Text: "<script>alert(1)</script>"}}}}, Total: 1}}
	w := serve(f, "/projects/p1/knowledge/search?q=script")
	var m struct {
		Items []struct {
			Snippet []base.Segment `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || m.Items[0].Snippet[0].Text != "<script>alert(1)</script>" {
		t.Fatalf("%v %s", err, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "<mark>") {
		t.Fatal("mark")
	}
}

func TestHandler_Errors(t *testing.T) {
	cases := map[string]struct {
		err  error
		code int
		c    string
	}{
		"404": {base.ErrProjectNotFound, 404, "NOT_FOUND"},
		"401": {base.ErrUnauthenticated, 401, "UNAUTHORIZED"},
		"400": {base.Invalid(base.ReasonInvalidLimit), 400, "INVALID_ARGUMENT"},
	}
	for name, tc := range cases {
		w := serve(&fakeExec{err: tc.err}, "/projects/x/knowledge/search?q=a")
		var m struct {
			Error struct {
				Code    string         `json:"code"`
				Details map[string]any `json:"details"`
			} `json:"error"`
			TraceID string `json:"trace_id"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		if w.Code != tc.code || m.Error.Code != tc.c {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
		if name == "400" && m.Error.Details["reason"] != "invalid_limit" {
			t.Fatalf("details: %v", m.Error.Details)
		}
	}
}

func TestHandler_JSONSectionAndExpanded(t *testing.T) {
	f := &fakeExec{res: base.SearchResultDTO{
		Items:    []base.SearchItemDTO{{Path: "a.md", Section: "Alur login", Snippet: []base.Segment{{Text: "x"}}}},
		Total:    1,
		Expanded: true,
	}}
	w := serve(f, "/projects/p1/knowledge/search?q=login")
	var m struct {
		Items    []map[string]any `json:"items"`
		Expanded bool             `json:"expanded"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if !m.Expanded || m.Items[0]["section"] != "Alur login" {
		t.Fatalf("%s", w.Body.String())
	}
}
