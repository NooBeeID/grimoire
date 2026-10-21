package deleteproject

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
)

type fakeExec struct {
	err   error
	calls int
	got   Input
}

func (f *fakeExec) Execute(_ context.Context, in Input) error {
	f.calls++
	f.got = in
	return f.err
}

func serve(f *fakeExec, target, body, ua string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Delete("/projects/{projectID}", (&Handler{uc: f}).ServeHTTP)
	req := httptest.NewRequest(http.MethodDelete, target, strings.NewReader(body))
	req.Header.Set("User-Agent", ua)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_204(t *testing.T) {
	f := &fakeExec{}
	w := serve(f, "/projects/p1", `{"confirm_name":"Pay"}`, strings.Repeat("ä", 600))
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	if f.got.ProjectID != "p1" || f.got.ConfirmName != "Pay" || f.got.Meta.IP == "" {
		t.Fatalf("%+v", f.got)
	}
	if n := len([]rune(f.got.Meta.UserAgent)); n != 512 {
		t.Fatalf("ua runes: %d", n)
	}
}

func TestHandler_BodyErrors(t *testing.T) {
	for _, body := range []string{"", "{rusak", `{"name":"x"}`, `{}`} {
		f := &fakeExec{}
		w := serve(f, "/projects/p1", body, "ua")
		if w.Code != 400 || f.calls != 0 || !strings.Contains(w.Body.String(), "INVALID_ARGUMENT") {
			t.Fatalf("%q: %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	f := &fakeExec{err: base.ErrProjectNotFound}
	if w := serve(f, "/projects/p1", `{"confirm_name":"x"}`, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	f.err = base.ErrForbidden
	if w := serve(f, "/projects/p1", `{"confirm_name":"x"}`, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	f.err = base.ErrConfirmationMismatch
	w := serve(f, "/projects/p1", `{"confirm_name":"x"}`, "")
	var m struct {
		Error struct{ Details map[string]any } `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 400 || m.Error.Details["reason"] != "CONFIRMATION_MISMATCH" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	f.err = errors.New("rahasia-internal")
	w = serve(f, "/projects/p1", `{"confirm_name":"x"}`, "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "rahasia-internal") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_ProjectIDPassedVerbatim(t *testing.T) {
	f := &fakeExec{}
	serve(f, "/projects/bukan-uuid", `{"confirm_name":"x"}`, "")
	if f.got.ProjectID != "bukan-uuid" {
		t.Fatal(f.got.ProjectID)
	}
}
