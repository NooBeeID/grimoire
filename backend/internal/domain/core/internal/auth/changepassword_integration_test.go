//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/changepassword"
)

const newPW = "password-baru-xyz-9"

// setupCP menyiapkan env login + endpoint change-password.
func setupCP(t *testing.T, o envOpts) *loginEnv {
	t.Helper()
	e := setupLogin(t, o)
	shared, err := buildShared(e.cfg.Config)
	if err != nil {
		t.Fatal(err)
	}
	shared.Clock = func() time.Time { return *e.now }
	changepassword.Init(e.cfg, shared)
	return e
}

func (e *loginEnv) loginCookie(email, pw string) *http.Cookie {
	e.t.Helper()
	w := e.login(email, pw)
	if w.Code != 200 {
		e.t.Fatalf("login %d %s", w.Code, w.Body)
	}
	return sessionCookie(w)
}

func (e *loginEnv) changePW(c *http.Cookie, cur, next string, csrf bool) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"current_password": cur, "new_password": next})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(string(b)))
	r.RemoteAddr = "10.0.0.9:4000"
	if csrf {
		r.Header.Set("X-Requested-With", "grimoire")
	}
	if c != nil {
		r.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *loginEnv) get(c *http.Cookie, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if c != nil {
		r.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func TestT01_AC4_MustChangePasswordGate(t *testing.T) {
	e := setupCP(t, envOpts{})
	e.seed("budi@tim.id", true, true)
	c := e.loginCookie("budi@tim.id", loginPW)

	// Endpoint non-exempt diblokir; route tak dikenal juga 403 (bukan 404).
	for _, p := range []string{"/api/_whoami", "/api/tidak-ada"} {
		w := e.get(c, p)
		if w.Code != 403 || errCode(t, w) != "PASSWORD_CHANGE_REQUIRED" {
			t.Fatalf("%s: %d %s", p, w.Code, w.Body)
		}
	}
	// Berganti password diizinkan.
	if w := e.changePW(c, loginPW, newPW, true); w.Code != 204 || w.Body.Len() != 0 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("change %d %s", w.Code, w.Body)
	}
	if w := e.get(c, "/api/_whoami"); w.Code != 200 {
		t.Fatalf("setelah ganti: %d %s", w.Code, w.Body)
	}
	if n := e.countRows(`SELECT count(*) FROM users WHERE email='budi@tim.id' AND NOT must_change_password`); n != 1 {
		t.Fatal("flag belum false")
	}
}

func TestT01_ChangePassword_OtherSessionsRevoked(t *testing.T) {
	e := setupCP(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	a := e.loginCookie("budi@tim.id", loginPW)
	b := e.loginCookie("budi@tim.id", loginPW)
	if w := e.changePW(a, loginPW, newPW, true); w.Code != 204 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := e.get(b, "/api/_whoami"); w.Code != 401 {
		t.Fatalf("sesi B harus 401: %d", w.Code)
	}
	if w := e.get(a, "/api/_whoami"); w.Code != 200 {
		t.Fatalf("sesi A harus 200: %d", w.Code)
	}
	if w := e.login("budi@tim.id", loginPW); w.Code != 401 || errCode(t, w) != "INVALID_CREDENTIALS" {
		t.Fatalf("password lama: %d %s", w.Code, w.Body)
	}
	if w := e.login("budi@tim.id", newPW); w.Code != 200 {
		t.Fatalf("password baru: %d %s", w.Code, w.Body)
	}
}

func TestT01_ChangePassword_ValidationMatrix(t *testing.T) {
	e := setupCP(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	a := e.loginCookie("budi@tim.id", loginPW)
	e.loginCookie("budi@tim.id", loginPW)
	var before string
	if err := e.d.GetContext(context.Background(), &before, `SELECT password_hash FROM users WHERE email='budi@tim.id'`); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, cur, next, field, msg string }{
		{"salah", "salah-salah-1", newPW, "current_password", "password lama salah"},
		{"pendek", loginPW, "sembilan9", "new_password", "minimal 10 karakter"},
		{"email", loginPW, "BUDI@tim.id", "new_password", "password tidak boleh sama dengan email"},
		{"sama", loginPW, loginPW, "new_password", "password baru harus berbeda dari password lama"},
	}
	for _, c := range cases {
		w := e.changePW(a, c.cur, c.next, true)
		if w.Code != 400 || errCode(t, w) != "INVALID_ARGUMENT" || !strings.Contains(w.Body.String(), c.msg) {
			t.Fatalf("%s: %d %s", c.name, w.Code, w.Body)
		}
	}
	var after string
	_ = e.d.GetContext(context.Background(), &after, `SELECT password_hash FROM users WHERE email='budi@tim.id'`)
	if after != before || e.countRows(`SELECT count(*) FROM sessions`) != 2 {
		t.Fatal("hash/sesi berubah pada jalur gagal")
	}
	// Sesi tetap sah setelah salah password lama (bukan 401).
	if w := e.get(a, "/api/_whoami"); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
}

func TestT01_ChangePassword_RateLimited(t *testing.T) {
	e := setupCP(t, envOpts{ratePerMinute: 4}) // burst 1
	e.seed("budi@tim.id", true, false)
	a := e.loginCookie("budi@tim.id", loginPW)
	if w := e.changePW(a, "salah-salah-1", newPW, true); w.Code != 400 {
		t.Fatalf("pertama: %d %s", w.Code, w.Body)
	}
	w := e.changePW(a, "salah-salah-1", newPW, true)
	if w.Code != 429 || errCode(t, w) != "RATE_LIMITED" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_ChangePassword_RequiresCSRFHeader(t *testing.T) {
	e := setupCP(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	a := e.loginCookie("budi@tim.id", loginPW)
	w := e.changePW(a, loginPW, newPW, false)
	if w.Code != 403 || errCode(t, w) != "CSRF_REQUIRED" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_ChangePassword_Anonymous401(t *testing.T) {
	e := setupCP(t, envOpts{})
	w := e.changePW(nil, loginPW, newPW, true)
	if w.Code != 401 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_ChangePassword_AuditAndNoSecretLeak(t *testing.T) {
	e := setupCP(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	a := e.loginCookie("budi@tim.id", loginPW)
	e.loginCookie("budi@tim.id", loginPW)

	// jalur gagal: nol audit.
	e.changePW(a, "salah-salah-1", newPW, true)
	if n := len(e.auditsFor("auth.password.changed")); n != 0 {
		t.Fatalf("audit pada jalur gagal: %d", n)
	}
	if w := e.changePW(a, loginPW, newPW, true); w.Code != 204 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := e.auditsFor("auth.password.changed")
	if len(got) != 1 || got[0].Target != "" && got[0].TargetType != "user" || got[0].Args["revoked_sessions"] != float64(1) && got[0].Args["revoked_sessions"] != int64(1) {
		t.Fatalf("%+v", got)
	}
	blob, _ := json.Marshal(got)
	all := string(blob) + e.logs.String()
	for _, s := range []string{loginPW, newPW, "salah-salah-1", a.Value, "$argon2id$"} {
		if strings.Contains(all, s) {
			t.Fatalf("rahasia bocor: %q", s)
		}
	}
}

func TestT01_ChangePassword_AuditUnregisteredDoesNotFail(t *testing.T) {
	e := setupCP(t, envOpts{noAudit: true})
	e.seed("budi@tim.id", true, false)
	a := e.loginCookie("budi@tim.id", loginPW)
	if w := e.changePW(a, loginPW, newPW, true); w.Code != 204 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(e.logs.String(), "audit record gagal") {
		t.Fatal("log audit gagal tidak ada")
	}
}
