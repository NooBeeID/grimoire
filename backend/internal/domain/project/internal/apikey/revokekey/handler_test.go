package revokekey

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/lib/common/reqctx"
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

// scopedRepo: repo in-memory yang memfilter seperti repo nyata.
type scopedRepo struct{ tx *fakeTx }

func (r *scopedRepo) InTx(_ context.Context, fn func(txRepo) error) error { return fn(r.tx) }

func e2eServe(uc *Usecase, id reqctx.Identity, keyID string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/api-keys/{keyID}/revoke", newHandler(uc).ServeHTTP)
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/api-keys/"+keyID+"/revoke", nil)
	req = req.WithContext(reqctx.WithIdentity(req.Context(), id))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func e2eRig(key base.Key) (*Usecase, *fakeTx, *fakeAudit) {
	l := &log{}
	tx := &fakeTx{l: l, key: key, found: true}
	au := &fakeAudit{l: l}
	uc := NewUsecase(&scopedRepo{tx: tx}, &fakeAccess{l: l}, au, func() time.Time { return fixedNow })
	return uc, tx, au
}

func normBody(t *testing.T, b []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "trace_id")
	out, _ := json.Marshal(m)
	return string(out)
}

func TestHandler_OtherUser404IdenticalToMissing(t *testing.T) {
	uc, tx, au := e2eRig(activeKey()) // milik u1
	idB := reqctx.Identity{UserID: "u2", Email: "b@tim.id", Role: "developer"}
	var bodies []string
	for _, kid := range []string{"k1", "00000000-0000-7000-8000-000000000000", "not-a-uuid"} {
		if kid != "k1" {
			tx.found = false
		}
		w := e2eServe(uc, idB, kid)
		if w.Code != 404 {
			t.Fatalf("%s code=%d", kid, w.Code)
		}
		bodies = append(bodies, normBody(t, w.Body.Bytes()))
		tx.found = true
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Fatalf("beda: %s vs %s", b, bodies[0])
		}
	}
	if !strings.Contains(bodies[0], "KEY_NOT_FOUND") {
		t.Fatalf("body %s", bodies[0])
	}
	if tx.markedCalls != 0 || len(au.events) != 0 {
		t.Fatal("key tersentuh / audit tercatat")
	}
}

func TestHandler_OwnerAndSuperAdmin(t *testing.T) {
	uc, _, au := e2eRig(activeKey())
	w := e2eServe(uc, reqctx.Identity{UserID: "u1", Email: "a@tim.id", Role: "developer"}, "k1")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"revoked"`) || len(au.events) != 1 {
		t.Fatalf("code=%d %s", w.Code, w.Body)
	}

	uc, _, au = e2eRig(otherKey())
	w = e2eServe(uc, reqctx.Identity{UserID: "su", Email: "su@tim.id", Role: "super_admin"}, "k1")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"revoked_by":"su"`) || au.events[0].Actor.ID != "su" {
		t.Fatalf("code=%d %s", w.Code, w.Body)
	}
}

func TestHandler_AlreadyRevokedOwn409(t *testing.T) {
	k := activeKey()
	at := created
	k.RevokedAt = &at
	uc, _, _ := e2eRig(k)
	w := e2eServe(uc, reqctx.Identity{UserID: "u1", Email: "a@tim.id", Role: "developer"}, "k1")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "ALREADY_REVOKED") {
		t.Fatalf("code=%d %s", w.Code, w.Body)
	}
}
