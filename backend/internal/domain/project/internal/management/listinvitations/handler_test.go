package listinvitations

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

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/errs"
)

type fakeExec struct {
	res Result
	err error
	got string
}

func (f *fakeExec) Execute(_ context.Context, id string) (Result, error) {
	f.got = id
	return f.res, f.err
}

func serve(f *fakeExec, target, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/projects/{projectID}/invitations", newHandler(f).ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_OK(t *testing.T) {
	label := "Owner"
	f := &fakeExec{res: Result{Items: []base.InvitationDTO{{
		UserID: "u1", Email: "a@x.id", Name: "a", Role: "member", InvitedByLabel: &label,
		AddedAt: time.Date(2026, 10, 21, 8, 15, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 10, 28, 8, 15, 0, 0, time.UTC),
		DeliveryStatus: "failed", EmailFailed: true, ResendCount: 1,
	}}}}
	w := serve(f, "/projects/p1/invitations?x=1", `{"junk":true}`)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") ||
		w.Header().Get("Cache-Control") != "no-store" || f.got != "p1" {
		t.Fatalf("%d %v %q", w.Code, w.Header(), f.got)
	}
	var m struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || len(m.Items) != 1 {
		t.Fatalf("%v %s", err, w.Body)
	}
	keys := []string{"user_id", "email", "name", "role", "invited_by_label", "added_at", "sent_at", "expires_at",
		"expired", "delivery_status", "email_failed", "resend_count"}
	for _, k := range keys {
		if _, ok := m.Items[0][k]; !ok {
			t.Fatalf("missing %s", k)
		}
	}
	if len(m.Items[0]) != len(keys) || m.Items[0]["sent_at"] != nil || m.Items[0]["email_failed"] != true {
		t.Fatalf("%v", m.Items[0])
	}
}

func TestHandler_EmptyItemsIsArray(t *testing.T) {
	for _, res := range []Result{{}, {Items: []base.InvitationDTO{}}} {
		w := serve(&fakeExec{res: res}, "/projects/p1/invitations", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{base.ErrProjectNotFound, 404},
		{base.ErrForbidden, 403},
		{errs.New(errs.CodeUnauthorized, "perlu login"), 401},
	}
	for _, c := range cases {
		if w := serve(&fakeExec{err: c.err}, "/projects/p1/invitations", ""); w.Code != c.code {
			t.Fatalf("%v: %d %s", c.err, w.Code, w.Body)
		}
	}
}

func TestHandler_InternalErrorNotLeaked(t *testing.T) {
	w := serve(&fakeExec{err: errors.New("secret-detail")}, "/projects/p1/invitations", "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret-detail") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
