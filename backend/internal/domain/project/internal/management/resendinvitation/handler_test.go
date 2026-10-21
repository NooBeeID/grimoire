package resendinvitation

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
	res   ResendResponse
	err   error
	calls int
	got   Input
}

func (f *fakeExec) Execute(_ context.Context, in Input) (ResendResponse, error) {
	f.calls++
	f.got = in
	return f.res, f.err
}

func serve(f *fakeExec, target, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/invitations/{userID}/resend", handler{uc: f}.ServeHTTP)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, target, strings.NewReader(body)))
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return m
}

func TestHandler_SuccessNoCredentials(t *testing.T) {
	f := &fakeExec{res: ResendResponse{EmailSent: true, EmailStatus: "sent", ExpiresAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}
	w := serve(f, "/projects/p1/invitations/u1/resend", "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	m := decode(t, w)
	if m["email_sent"] != true || m["email_status"] != "sent" || m["expires_at"] == nil {
		t.Fatalf("%v", m)
	}
	if _, ok := m["credentials"]; ok {
		t.Fatal("credentials tidak boleh ada")
	}
	if w.Header().Get("Cache-Control") == "no-store" {
		t.Fatal("no-store tidak boleh ada")
	}
}

func TestHandler_SuccessWithCredentials(t *testing.T) {
	f := &fakeExec{res: ResendResponse{EmailStatus: "failed", Credentials: &CredentialsResponse{
		Email: "t@x.id", TemporaryPassword: "pw-asli", LoginURL: "https://app/login"}}}
	w := serve(f, "/projects/p1/invitations/u1/resend", "")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	c, _ := decode(t, w)["credentials"].(map[string]any)
	if c["email"] != "t@x.id" || c["temporary_password"] != "pw-asli" || c["login_url"] != "https://app/login" {
		t.Fatalf("%v", c)
	}
}

func TestHandler_ParamsPassedAsIs(t *testing.T) {
	f := &fakeExec{}
	serve(f, "/projects/p1/invitations/bukan-uuid/resend", "")
	if f.got.ProjectID != "p1" || f.got.UserID != "bukan-uuid" {
		t.Fatalf("%+v", f.got)
	}
}

func TestHandler_BodyIgnored(t *testing.T) {
	f := &fakeExec{}
	w := serve(f, "/projects/p1/invitations/u1/resend", "{bukan json !!")
	if w.Code != 200 || f.calls != 1 {
		t.Fatalf("%d", w.Code)
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{"project 404", base.ErrProjectNotFound, 404, ""},
		{"member 404", base.ErrMemberNotFound, 404, ""},
		{"forbidden", base.ErrForbidden, 403, ""},
		{"not pending", base.ErrInvitationNotPending, 409, "reason"},
		{"rate limited", errs.WithDetails(errs.CodeRateLimited, "sering", map[string]any{"retry_after_seconds": 30, "scope": "resend"}), 429, "retry_after_seconds"},
		{"already registered", errs.WithDetails(errs.CodeAlreadyRegistered, "nonaktif", map[string]any{"reason": "inactive"}), 409, "reason"},
		{"random", errors.New("rahasia internal"), 500, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := serve(&fakeExec{err: c.err}, "/projects/p1/invitations/u1/resend", "")
			if w.Code != c.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") == "no-store" {
				t.Fatal("no-store palsu pada error")
			}
			if strings.Contains(w.Body.String(), "temporary_password") {
				t.Fatal("body error memuat kredensial")
			}
			if c.status == 500 && strings.Contains(w.Body.String(), "rahasia internal") {
				t.Fatal("pesan internal bocor")
			}
			if c.detail != "" && !strings.Contains(w.Body.String(), c.detail) {
				t.Fatalf("details %q hilang: %s", c.detail, w.Body.String())
			}
			if c.name == "rate limited" && !strings.Contains(w.Body.String(), "resend") {
				t.Fatalf("scope hilang: %s", w.Body.String())
			}
		})
	}
}
