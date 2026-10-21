package revokekey

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apikey/base"
)

type fakeUC struct {
	in  Input
	dto base.KeyDTO
	err error
}

func (f *fakeUC) Execute(_ context.Context, in Input) (base.KeyDTO, error) {
	f.in = in
	return f.dto, f.err
}

func serve(f *fakeUC, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/api-keys/{keyID}/revoke", newHandler(f).ServeHTTP)
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/api-keys/k1/revoke", strings.NewReader(body))
	req.Header.Set("User-Agent", "agent/1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_Success(t *testing.T) {
	f := &fakeUC{dto: base.KeyDTO{ID: "k1", Name: "ci", Prefix: "abcd1234", Status: "revoked"}}
	w := serve(f, "not json")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("code=%d ct=%s", w.Code, w.Header().Get("Content-Type"))
	}
	if f.in.ProjectID != "p1" || f.in.KeyID != "k1" || f.in.Meta.UserAgent != "agent/1" {
		t.Fatalf("in=%+v", f.in)
	}
	var out map[string]map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["key"]["status"] != "revoked" {
		t.Fatalf("body=%s", w.Body)
	}
	for _, k := range []string{"secret", "secret_hash", "hash", "raw_key"} {
		if _, ok := out["key"][k]; ok {
			t.Fatalf("bocor %s", k)
		}
	}
}

func TestHandler_Errors(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		reason string
		msg    string
	}{
		"key":      {base.ErrKeyNotFound, 404, "KEY_NOT_FOUND", ""},
		"project":  {base.ErrProjectNotFound, 404, "", "project tidak ditemukan"},
		"revoked":  {base.ErrAlreadyRevoked, 409, "ALREADY_REVOKED", ""},
		"internal": {errors.New("pq: secret internal detail"), 500, "", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			w := serve(&fakeUC{err: c.err}, "")
			if w.Code != c.status {
				t.Fatalf("code=%d body=%s", w.Code, w.Body)
			}
			body := w.Body.String()
			if c.reason != "" && !strings.Contains(body, c.reason) {
				t.Fatalf("body=%s", body)
			}
			if c.msg != "" && !strings.Contains(body, c.msg) {
				t.Fatalf("body=%s", body)
			}
			if c.status == 500 && strings.Contains(body, "internal detail") {
				t.Fatalf("bocor: %s", body)
			}
			if !strings.Contains(body, "trace_id") {
				t.Fatalf("tanpa trace_id: %s", body)
			}
		})
	}
}
