package inviteuser

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
)

func serve(e env, body string, id *reqctx.Identity) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/users/invite", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-ua")
	if id != nil {
		req = req.WithContext(reqctx.WithIdentity(req.Context(), *id))
	}
	w := httptest.NewRecorder()
	NewHandler(e.uc).Invite(w, req)
	return w
}

var admin = &reqctx.Identity{UserID: "admin-1", Email: "admin@tim.id", Name: "Admin Satu", Role: base.RoleSuperAdmin}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, w.Body)
	}
	return m
}

func errBody(t *testing.T, w *httptest.ResponseRecorder) (string, map[string]any) {
	t.Helper()
	e := decode(t, w)["error"].(map[string]any)
	d, _ := e["details"].(map[string]any)
	return e["code"].(string), d
}

func TestHandler_Created201_EmailSent(t *testing.T) {
	e := setup()
	w := serve(e, `{"email":" Baru@Contoh.id ","name":""}`, admin)
	if w.Code != 201 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("code=%d h=%v", w.Code, w.Header())
	}
	if w.Header().Get("Cache-Control") == "no-store" {
		t.Fatal("no-store hanya untuk fallback")
	}
	m := decode(t, w)
	if _, ok := m["credentials"]; ok {
		t.Fatalf("kunci credentials tidak boleh ada: %s", w.Body)
	}
	u := m["user"].(map[string]any)
	inv := u["invitation"].(map[string]any)
	if m["email_sent"] != true || m["email_status"] != "sent" || m["created"] != true ||
		inv["delivery_status"] != "sent" || inv["pending"] != true || inv["expired"] != false ||
		u["email"] != "baru@contoh.id" || u["name"] != "baru" || u["role"] != "developer" || u["id"] != "new-id" {
		t.Fatalf("body: %s", w.Body)
	}
	if _, ok := u["password_hash"]; ok {
		t.Fatal("hash bocor")
	}
	if e.a.events[0].Meta.UserAgent != "test-ua" {
		t.Fatalf("meta: %+v", e.a.events[0].Meta)
	}
}

func TestHandler_Created201_FallbackNoStore(t *testing.T) {
	for _, st := range []string{base.DeliveryFailed, base.DeliveryNotConfigured} {
		e := setup()
		e.s.status = st
		w := serve(e, `{"email":"a@x.id"}`, admin)
		if w.Code != 201 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("code=%d h=%v", w.Code, w.Header())
		}
		m := decode(t, w)
		c, _ := m["credentials"].(map[string]any)
		if m["email_sent"] != false || m["email_status"] != st || c["email"] != "a@x.id" || len(c["temporary_password"].(string)) != 16 || c["login_url"] != "https://g.id/login" {
			t.Fatalf("body: %s", w.Body)
		}
	}
}

func TestHandler_Anonymous401(t *testing.T) {
	e := setup()
	w := serve(e, `{"email":"a@x.id"}`, nil)
	if c, _ := errBody(t, w); w.Code != 401 || c != "UNAUTHORIZED" || e.r.findN != 0 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestHandler_Developer403ViaRouter(t *testing.T) {
	e := setup()
	h := middleware.RequireRole(base.RoleSuperAdmin)(http.HandlerFunc(NewHandler(e.uc).Invite))
	req := httptest.NewRequest(http.MethodPost, "/api/users/invite", strings.NewReader(`{"email":"a@x.id"}`))
	req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "d", Role: base.RoleDeveloper}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if c, _ := errBody(t, w); w.Code != 403 || c != "FORBIDDEN" || e.r.findN != 0 || e.r.insertN != 0 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestHandler_InvalidEmail400(t *testing.T) {
	w := serve(setup(), `{"email":"bukan-email"}`, admin)
	c, d := errBody(t, w)
	f, _ := d["fields"].(map[string]any)
	if w.Code != 400 || c != "INVALID_ARGUMENT" || f["email"] == nil {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestHandler_DecodeErrors400(t *testing.T) {
	cases := map[string]string{
		"role":      `{"email":"a@x.id","role":"super_admin"}`,
		"password":  `{"email":"a@x.id","password":"rahasia-rahasia"}`,
		"is_active": `{"email":"a@x.id","is_active":false}`,
		"empty":     ``,
		"malformed": `{bad`,
		"wrong":     `{"email":5}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			e := setup()
			w := serve(e, body, admin)
			if c, _ := errBody(t, w); w.Code != 400 || c != "INVALID_ARGUMENT" || e.r.findN != 0 || e.r.insertN != 0 {
				t.Fatalf("code=%d body=%s", w.Code, w.Body)
			}
		})
	}
}

func TestHandler_AlreadyRegistered409(t *testing.T) {
	e := setup()
	e.r.user = &base.User{ID: "u", Role: base.RoleDeveloper, IsActive: true}
	w := serve(e, `{"email":"a@x.id"}`, admin)
	c, d := errBody(t, w)
	if w.Code != 409 || c != "ALREADY_REGISTERED" || d["reason"] != "registered" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestHandler_RateLimited429(t *testing.T) {
	e := setup()
	e.r.count, e.r.oldest = 20, t0
	w := serve(e, `{"email":"a@x.id"}`, admin)
	c, d := errBody(t, w)
	if w.Code != 429 || c != "RATE_LIMITED" || d["scope"] != "invite" || d["retry_after_seconds"] == nil {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestHandler_InternalErrorHidesDetail(t *testing.T) {
	e := setup()
	e.r.insertErr = errors.New("pq: connection refused smtp password=zzz")
	w := serve(e, `{"email":"a@x.id"}`, admin)
	c, _ := errBody(t, w)
	if w.Code != 500 || c != "INTERNAL" || strings.Contains(w.Body.String(), "pq:") || strings.Contains(w.Body.String(), "zzz") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestHandler_LabelFallsBackToEmail(t *testing.T) {
	e := setup()
	id := &reqctx.Identity{UserID: "admin-1", Email: "admin@tim.id", Role: base.RoleSuperAdmin}
	if w := serve(e, `{"email":"a@x.id"}`, id); w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if e.r.inv.InvitedByLabel == nil || *e.r.inv.InvitedByLabel != "admin@tim.id" {
		t.Fatalf("label: %v", e.r.inv.InvitedByLabel)
	}
	e = setup()
	serve(e, `{"email":"a@x.id"}`, admin)
	if *e.r.inv.InvitedByLabel != "Admin Satu" {
		t.Fatalf("label: %v", *e.r.inv.InvitedByLabel)
	}
}

func TestHandler_NoPasswordInLogsOnError(t *testing.T) {
	e := setup()
	serve(e, `{"email":"a@x.id","password":"super-rahasia-123"}`, admin)
	if strings.Contains(e.logs.String(), "super-rahasia-123") {
		t.Fatal("bocor di log")
	}
}
