package login

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/lib/common/errs"
)

func init() {
	httpx.RegisterCodeStatus(base.CodeInvalidCredentials, http.StatusUnauthorized)
	httpx.RegisterCodeStatus(base.CodeAccountLocked, http.StatusTooManyRequests)
}

func doLogin(f *fixture, secure bool, body string, ua string) *httptest.ResponseRecorder {
	h := NewHandler(f.uc, secure)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:1234"
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	}
	w := httptest.NewRecorder()
	h.Login(w, r)
	return w
}

const okBody = `{"email":"budi@tim.id","password":"` + goodPW + `"}`

func TestLoginHandler_SetsCookieAttributes(t *testing.T) {
	for _, secure := range []bool{true, false} {
		f := newFixture(t)
		w := doLogin(f, secure, okBody, "")
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		cs := w.Result().Cookies()
		if len(cs) != 1 {
			t.Fatalf("cookies %v", cs)
		}
		c := cs[0]
		if c.Name != "grimoire_session" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" ||
			c.MaxAge != 604800 || c.Secure != secure || len(c.Value) != 43 {
			t.Fatalf("%+v", c)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("no-store")
		}
		var out LoginResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.User.Email != "budi@tim.id" || !out.User.MustChangePassword {
			t.Fatalf("%v %s", err, w.Body)
		}
		if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "argon2") {
			t.Fatal("hash bocor")
		}
	}
}

func TestLoginHandler_InvalidJSON(t *testing.T) {
	for _, body := range []string{`bukan json`, `{"email":"a","password":"b","x":1}`, `{"email":1,"password":"b"}`, ``} {
		f := newFixture(t)
		w := doLogin(f, true, body, "")
		if w.Code != 400 || len(w.Result().Cookies()) != 0 || !strings.Contains(w.Body.String(), "INVALID_ARGUMENT") {
			t.Fatalf("%q -> %d %s", body, w.Code, w.Body)
		}
	}
}

func TestLoginHandler_ValidationFieldErrors(t *testing.T) {
	f := newFixture(t)
	w := doLogin(f, true, `{"email":"","password":""}`, "")
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"email":"wajib diisi"`) || !strings.Contains(w.Body.String(), `"password":"wajib diisi"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestLoginHandler_LockedSetsRetryAfter(t *testing.T) {
	f := newFixture(t)
	until := testNow.Add(812 * time.Second)
	f.repo.throttle.LockedUntil = &until
	w := doLogin(f, true, okBody, "")
	if w.Code != 429 || w.Header().Get("Retry-After") != "812" || !strings.Contains(w.Body.String(), "ACCOUNT_LOCKED") ||
		!strings.Contains(w.Body.String(), `"retry_after_seconds":812`) {
		t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body)
	}
}

func TestLoginHandler_FailureHasNoCookie(t *testing.T) {
	f := newFixture(t)
	w := doLogin(f, true, `{"email":"budi@tim.id","password":"salah-salah-1"}`, "")
	if w.Code != 401 || len(w.Result().Cookies()) != 0 || w.Header().Get("Retry-After") != "" ||
		!strings.Contains(w.Body.String(), "INVALID_CREDENTIALS") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestLoginHandler_UserAgentTruncated(t *testing.T) {
	f := newFixture(t)
	ua := strings.Repeat("a", 2000)
	if w := doLogin(f, true, okBody, ua); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if got := f.repo.completed[0].UserAgent; got == nil || len(*got) != 512 {
		t.Fatalf("ua %v", got)
	}
	// multibyte: 511 'a' + "é" (2 byte) melewati batas -> dipotong sebelum é
	if got := truncate(strings.Repeat("a", 511)+"é", 512); got != strings.Repeat("a", 511) {
		t.Fatalf("len=%d", len(got))
	}
	if truncate("pendek", 512) != "pendek" {
		t.Fatal("tidak boleh berubah")
	}
}

func TestLoginRequest_PasswordNotPrinted(t *testing.T) {
	req := LoginRequest{Email: "a@b.c", Password: "rahasia-asli"}
	b, _ := json.Marshal(req)
	var sb strings.Builder
	slog.New(slog.NewJSONHandler(&sb, nil)).Info("x", "req", req)
	for _, s := range []string{string(b), fmt.Sprintf("%v %+v %#v", req, req, req), sb.String()} {
		if strings.Contains(s, "rahasia-asli") {
			t.Fatal(s)
		}
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	if n, ok := retryAfterSeconds(base.AccountLocked(5 * time.Second)); !ok || n != 5 {
		t.Fatal(n, ok)
	}
	for _, err := range []error{
		fmt.Errorf("lain"), base.ErrInvalidCreds,
		errs.WithDetails(base.CodeAccountLocked, "x", "bukan map"),
		errs.WithDetails(base.CodeAccountLocked, "x", map[string]any{"retry_after_seconds": "5"}),
		errs.WithDetails(base.CodeAccountLocked, "x", map[string]any{"retry_after_seconds": 0}),
		errs.WithDetails(errs.CodeInvalid, "x", map[string]any{"retry_after_seconds": 5}),
	} {
		if _, ok := retryAfterSeconds(err); ok {
			t.Fatalf("harus false: %v", err)
		}
	}
}
