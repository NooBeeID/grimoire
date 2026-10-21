package listtree

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

type fakeExec struct {
	lvl base.Level
	err error
	got Input
}

func (f *fakeExec) Execute(_ context.Context, in Input) (base.Level, error) {
	f.got = in
	return f.lvl, f.err
}

func serve(f *fakeExec, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/projects/{projectID}/knowledge/tree", newHandler(f).ServeHTTP)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func TestHandler_NoPrefix(t *testing.T) {
	f := &fakeExec{lvl: base.Level{Folders: []base.FolderNode{}, Files: []base.FileNode{}}}
	w := serve(f, "/projects/p1/knowledge/tree")
	if w.Code != 200 || f.got != (Input{ProjectID: "p1"}) {
		t.Fatalf("%d %+v", w.Code, f.got)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["prefix"] != "" || m["total_files"] != float64(0) {
		t.Fatalf("%v", m)
	}
}

func TestHandler_PrefixDecoded(t *testing.T) {
	f := &fakeExec{}
	serve(f, "/projects/p1/knowledge/tree?prefix=docs%2Fauth")
	if f.got.Prefix != "docs/auth" {
		t.Fatalf("%+v", f.got)
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
		"400": {base.Invalid(base.ReasonPathTraversal), 400, "INVALID_ARGUMENT"},
	}
	for name, tc := range cases {
		w := serve(&fakeExec{err: tc.err}, "/projects/x/knowledge/tree")
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
		if name == "400" && m.Error.Details["reason"] != "path_traversal" {
			t.Fatalf("details=%v", m.Error.Details)
		}
	}
}
