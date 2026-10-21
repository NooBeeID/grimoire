package changepassword

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
)

func doReq(e *env, body string, cookie bool) *httptest.ResponseRecorder {
	h := NewHandler(e.uc, []byte("secret"))
	r := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(body))
	r = r.WithContext(e.ctx)
	if cookie {
		r.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: "tok"})
	}
	w := httptest.NewRecorder()
	h.ChangePassword(w, r)
	return w
}

func TestHandler_Success204NoBodyNoCookie(t *testing.T) {
	e := setup(t)
	w := doReq(e, `{"current_password":"`+oldPW+`","new_password":"`+newPW+`"}`, true)
	if w.Code != 204 || w.Body.Len() != 0 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("%d %q", w.Code, w.Body)
	}
}

func TestHandler_HashesCurrentCookie(t *testing.T) {
	e := setup(t)
	doReq(e, `{"current_password":"`+oldPW+`","new_password":"`+newPW+`"}`, true)
	if string(e.repo.got.KeepTokenHash) != string(base.HashToken([]byte("secret"), "tok")) {
		t.Fatal("hash token salah")
	}
}

func TestHandler_NoCookie401(t *testing.T) {
	e := setup(t)
	if w := doReq(e, `{"current_password":"a","new_password":"b"}`, false); w.Code != 401 || e.repo.calls != 0 {
		t.Fatalf("%d", w.Code)
	}
}

func TestHandler_BadJSON400(t *testing.T) {
	e := setup(t)
	if w := doReq(e, `{bad`, true); w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

func TestHandler_UnknownFieldRejected(t *testing.T) {
	e := setup(t)
	if w := doReq(e, `{"current_password":"a","new_password":"b","x":1}`, true); w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

func TestHandler_ValidationErrorBodyHasNoSecrets(t *testing.T) {
	e := setup(t)
	w := doReq(e, `{"current_password":"salah-salah-1","new_password":"pendek"}`, true)
	if w.Code != 400 || strings.Contains(w.Body.String(), "salah-salah-1") || !strings.Contains(w.Body.String(), "password lama salah") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	_ = reqctx.Identity{}
}

func TestHandler_ExpiredInvitation400(t *testing.T) {
	e := setup(t)
	e.repo.hasInv = true
	e.repo.inv = base.Invitation{UserID: "u1", ExpiresAt: time.Now().Add(-time.Minute)}
	w := doReq(e, `{"current_password":"`+oldPW+`","new_password":"`+newPW+`"}`, true)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "INVALID_ARGUMENT") ||
		!strings.Contains(w.Body.String(), msgInvitationExpired) || !strings.Contains(w.Body.String(), `"current_password"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandler_PendingInvitationSuccess204(t *testing.T) {
	e := setup(t)
	e.repo.hasInv = true
	e.repo.accepted = true
	e.repo.inv = base.Invitation{UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)}
	w := doReq(e, `{"current_password":"`+oldPW+`","new_password":"`+newPW+`"}`, true)
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("%d %q", w.Code, w.Body)
	}
}
