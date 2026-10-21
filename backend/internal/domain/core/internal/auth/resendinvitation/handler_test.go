package resendinvitation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/invitation"
	contract "grimoire/lib/contracts/core/auth/resendinvitation"
)

func router(e *env) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
			next.ServeHTTP(w, rq.WithContext(reqctx.WithIdentity(rq.Context(), reqctx.Identity{UserID: "admin-1", Name: "Admin Satu", Role: base.RoleSuperAdmin})))
		})
	})
	r.Post("/users/{id}/resend-invitation", NewHandler(e.uc).Resend)
	return r
}

func post(h http.Handler, id, body string) *httptest.ResponseRecorder {
	rq := httptest.NewRequest(http.MethodPost, "/users/"+id+"/resend-invitation", strings.NewReader(body))
	rq.Header.Set("User-Agent", "ua-test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, rq)
	return w
}

func body(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body: %s", w.Body.String())
	}
	return m
}

func TestH1_SentNoCredentials(t *testing.T) {
	e := setup()
	w := post(router(e), uid, "")
	m := body(t, w)
	if w.Code != 200 || m["email_sent"] != true || m["email_status"] != "sent" || m["expires_at"] == nil {
		t.Fatalf("%d %v", w.Code, m)
	}
	if _, has := m["credentials"]; has || w.Header().Get("Cache-Control") != "" {
		t.Fatalf("credentials/no-store tidak boleh ada: %v %v", m, w.Header())
	}
}

func TestH2_FallbackCredentials(t *testing.T) {
	e := setup()
	e.mail.NotConfigured = true
	w := post(router(e), uid, "")
	m := body(t, w)
	c, _ := m["credentials"].(map[string]any)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || c == nil {
		t.Fatalf("%d %v", w.Code, m)
	}
	pw, _ := c["temporary_password"].(string)
	if pw == "***" || len(pw) != base.GeneratedPasswordLen || c["email"] != "baru@contoh.id" || c["login_url"] == "" {
		t.Fatalf("%v", c)
	}
	if m["email_status"] != "not_configured" || m["email_sent"] != false {
		t.Fatalf("%v", m)
	}
}

func TestH3_H4_BodyIgnoredParamForwarded(t *testing.T) {
	e := setup()
	h := router(e)
	for _, b := range []string{"", `{"x":1}`, "bukan json {{"} {
		e = setup()
		h = router(e)
		if w := post(h, uid, b); w.Code != 200 {
			t.Fatalf("%q: %d %s", b, w.Code, w.Body.String())
		}
		ev := e.rec.evs[0]
		if ev.Action != record.ActionUserInvitationResent || ev.Args["via"] != "users" || ev.Target != uid {
			t.Fatalf("%+v", ev)
		}
		if e.rp.inv.ResendCount != 1 {
			t.Fatal("Initial harus false")
		}
		if ev.Meta.UserAgent != "ua-test" {
			t.Fatalf("%+v", ev.Meta)
		}
	}
}

func TestH5_ErrorMapping(t *testing.T) {
	e := setup()
	h := router(e)
	if w := post(h, "bukan-uuid", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	e.rp.inv = nil
	w := post(h, uid, "")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "INVITATION_NOT_PENDING") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	e = setup()
	e.rp.user.IsActive = false
	w = post(router(e), uid, "")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "inactive") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	e = setup()
	h = router(e)
	for i := 0; i < 3; i++ {
		post(h, uid, "")
	}
	w = post(h, uid, "")
	if w.Code != 429 || !strings.Contains(w.Body.String(), "retry_after_seconds") || !strings.Contains(w.Body.String(), "resend") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	e = setup()
	e.rp.issueErr = errors.New("sql: rahasia-internal")
	w = post(router(e), uid, "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "rahasia-internal") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestB1_BusInitial(t *testing.T) {
	e := setup()
	h := NewBusHandler(e.uc)
	out, err := h.Handle(context.Background(), contract.Request{UserID: uid, InvitedBy: "owner-1", InvitedByLabel: "Owner Satu", ProjectName: "Atlas", Initial: true})
	if err != nil {
		t.Fatal(err)
	}
	r, ok := out.(contract.Response)
	if !ok || !r.EmailSent || r.EmailStatus != invitation.EmailStatusSent || r.Credentials != nil || r.ExpiresAt.IsZero() {
		t.Fatalf("%#v", out)
	}
	if len(e.rec.evs) != 0 || e.rp.inv.ResendCount != 0 {
		t.Fatal("Initial: tanpa audit resent/kuota")
	}
	// gagal kirim -> credentials terisi, via=project
	e = setup()
	e.mail.Err = errors.New("550")
	out, err = NewBusHandler(e.uc).Handle(context.Background(), contract.Request{UserID: uid, InvitedBy: "owner-1"})
	r = out.(contract.Response)
	if err != nil || r.EmailSent || r.Credentials == nil || r.Credentials.TemporaryPassword == "" || r.EmailStatus != "failed" {
		t.Fatalf("%v %#v", err, r)
	}
	if e.rec.evs[0].Args["via"] != "project" {
		t.Fatalf("%v", e.rec.evs[0].Args)
	}
}

func TestB2_WrongRequestType(t *testing.T) {
	e := setup()
	if _, err := NewBusHandler(e.uc).Handle(context.Background(), &contract.Request{UserID: uid}); err == nil {
		t.Fatal("pointer harus ditolak")
	}
	if _, err := NewBusHandler(e.uc).Handle(context.Background(), "x"); err == nil {
		t.Fatal("tipe salah harus ditolak")
	}
}

func TestB3_ErrsPassedThrough(t *testing.T) {
	e := setup()
	e.rp.inv = nil
	_, err := NewBusHandler(e.uc).Handle(context.Background(), contract.Request{UserID: uid})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict || reason(ee) != "INVITATION_NOT_PENDING" {
		t.Fatalf("%v", err)
	}
}
