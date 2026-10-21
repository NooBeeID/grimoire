package inviteuser

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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
	res   InviteResponse
	err   error
	calls int
	pid   string
	in    InviteRequest
	meta  base.RequestMeta
}

func (f *fakeExec) Execute(_ context.Context, p string, in InviteRequest, m base.RequestMeta) (InviteResponse, error) {
	f.calls++
	f.pid, f.in, f.meta = p, in, m
	return f.res, f.err
}

func serve(f *fakeExec, target, body, ua string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/invitations", newHandler(f).ServeHTTP)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("User-Agent", ua)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sampleResponse() InviteResponse {
	name, email, by, label := "Budi", "baru@contoh.id", "a", "owner@contoh.id"
	exp := time.Date(2026, 10, 28, 8, 15, 0, 0, time.UTC)
	return InviteResponse{
		Member: base.MemberDTO{UserID: "u", Name: &name, Email: &email, Found: true, IsActive: true, Role: "member",
			AddedAt: exp, AddedBy: &by, AddedByLabel: &label, InvitationPending: true, InvitationExpiresAt: &exp},
		Created: true, EmailSent: true, EmailStatus: "sent", ExpiresAt: exp,
	}
}

func TestHandler_201_NoCredentials(t *testing.T) {
	f := &fakeExec{res: sampleResponse()}
	w := serve(f, "/projects/p1/invitations", `{"email":" Baru@Contoh.id "}`, strings.Repeat("ä", 600))
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	b := w.Body.String()
	for _, k := range []string{`"invitation_pending":true`, `"invitation_expires_at":"2026-10-28T08:15:00Z"`, `"email_sent":true`, `"email_status":"sent"`, `"expires_at":"2026-10-28T08:15:00Z"`, `"created":true`} {
		if !strings.Contains(b, k) {
			t.Fatalf("missing %s: %s", k, b)
		}
	}
	if strings.Contains(b, "credentials") || w.Header().Get("Cache-Control") == "no-store" {
		t.Fatalf("credentials/no-store tidak boleh ada: %s %v", b, w.Header())
	}
	if f.pid != "p1" || f.in.Email != "baru@contoh.id" || f.meta.IP == "" || len([]rune(f.meta.UserAgent)) != 512 {
		t.Fatalf("%+v %+v", f.in, f.meta)
	}
}

func TestHandler_201_WithCredentials(t *testing.T) {
	res := sampleResponse()
	res.EmailSent, res.EmailStatus = false, "not_configured"
	res.Credentials = &CredentialsDTO{Email: "baru@contoh.id", TemporaryPassword: "pw-rahasia", LoginURL: "https://x/login"}
	w := serve(&fakeExec{res: res}, "/projects/p1/invitations", `{"email":"baru@contoh.id"}`, "ua")
	if w.Code != 201 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	for _, k := range []string{`"temporary_password":"pw-rahasia"`, `"login_url":"https://x/login"`, `"email":"baru@contoh.id"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("missing %s: %s", k, w.Body.String())
		}
	}
}

func TestHandler_BodyErrors(t *testing.T) {
	for _, body := range []string{"", "{rusak", `{"emailAddress":"x"}`, `{"email":5}`, `{"email":""}`, `{"email":"  "}`} {
		f := &fakeExec{}
		w := serve(f, "/projects/p1/invitations", body, "ua")
		if w.Code != 400 || f.calls != 0 || !strings.Contains(w.Body.String(), "INVALID_ARGUMENT") {
			t.Fatalf("%q: %d %s", body, w.Code, w.Body.String())
		}
	}
	w := serve(&fakeExec{}, "/projects/p1/invitations", `{"email":""}`, "ua")
	if !strings.Contains(w.Body.String(), `"email":"wajib diisi"`) {
		t.Fatal(w.Body.String())
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		code   int
		substr string
	}{
		{base.ErrForbidden, 403, "FORBIDDEN"},
		{base.ErrProjectNotFound, 404, "NOT_FOUND"},
		{base.ErrAlreadyMember, 409, "ALREADY_MEMBER"},
		{errs.WithDetails(errs.CodeAlreadyRegistered, "email sudah terdaftar", map[string]any{"reason": "registered"}), 409, "ALREADY_REGISTERED"},
		{errs.WithDetails(errs.CodeRateLimited, "terlalu banyak", map[string]any{"retry_after_seconds": 1840, "scope": "invite"}), 429, "retry_after_seconds"},
		{errors.New("secret db detail"), 500, "INTERNAL"},
	}
	for _, c := range cases {
		w := serve(&fakeExec{err: c.err}, "/projects/p1/invitations", `{"email":"a@x.id"}`, "ua")
		b := w.Body.String()
		if w.Code != c.code || !strings.Contains(b, c.substr) || strings.Contains(b, "secret") || !strings.Contains(b, "trace_id") {
			t.Fatalf("%v: %d %s", c.err, w.Code, b)
		}
	}
	// pembungkus %w tetap menghasilkan kode asli
	wrapped := errors.Join(errors.New("x"), errs.WithDetails(errs.CodeAlreadyRegistered, "m", map[string]any{"reason": "inactive"}))
	w := serve(&fakeExec{err: wrapped}, "/projects/p1/invitations", `{"email":"a@x.id"}`, "ua")
	if w.Code != 409 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_DoesNotLogBody(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	serve(&fakeExec{res: sampleResponse()}, "/projects/p1/invitations", `{"email":"rahasia@x.id"}`, "ua")
	if strings.Contains(buf.String(), "rahasia@x.id") {
		t.Fatal(buf.String())
	}
}
