package removemember

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
	r.Delete("/projects/{projectID}/members/{userID}", handler{uc: f}.ServeHTTP)
	req := httptest.NewRequest(http.MethodDelete, target, strings.NewReader(body))
	req.Header.Set("User-Agent", ua)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_204(t *testing.T) {
	f := &fakeExec{}
	w := serve(f, "/projects/p1/members/bukan-uuid", `{"x":1}`, strings.Repeat("ä", 600))
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	if f.got.ProjectID != "p1" || f.got.UserID != "bukan-uuid" || f.got.Meta.IP == "" {
		t.Fatalf("%+v", f.got)
	}
	if n := len([]rune(f.got.Meta.UserAgent)); n != 512 {
		t.Fatalf("ua runes: %d", n)
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{base.ErrProjectNotFound, 404, "NOT_FOUND"},
		{base.ErrMemberNotFound, 404, "NOT_FOUND"},
		{base.ErrForbidden, 403, "FORBIDDEN"},
		{base.ErrLastOwner, 409, "CONFLICT"},
		{errors.New("pq: rahasia"), 500, "INTERNAL"},
	}
	for _, c := range cases {
		w := serve(&fakeExec{err: c.err}, "/projects/p1/members/u1", "", "ua")
		var body struct {
			Error struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%v: %v", c.err, err)
		}
		if w.Code != c.status || body.Error.Code != c.code {
			t.Fatalf("%v: %d %s", c.err, w.Code, w.Body.String())
		}
		if c.status == 500 && strings.Contains(w.Body.String(), "rahasia") {
			t.Fatal("detail internal bocor")
		}
		if c.err == base.ErrMemberNotFound && body.Error.Message != "anggota tidak ditemukan" {
			t.Fatal(body.Error.Message)
		}
		if c.err == base.ErrLastOwner && body.Error.Details["reason"] != "LAST_OWNER" {
			t.Fatalf("%+v", body.Error.Details)
		}
	}
}
