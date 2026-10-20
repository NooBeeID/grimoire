package logout

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/middleware"
)

func newH(n int64, err error, secure bool) (*Handler, *fakeRepo, *fakeRec) {
	r := &fakeRepo{n: n, err: err}
	rec := &fakeRec{}
	return NewHandler(NewUsecase(r, rec, base.SessionConfig{Secret: secret}), secure), r, rec
}

func req(cookie bool) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	r.Header.Set("User-Agent", "ua-test")
	if cookie {
		r.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: "tok"})
	}
	return r
}

func TestHandler_ClearsCookie204(t *testing.T) {
	h, r, rec := newH(1, nil, true)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req(true))
	if w.Code != 204 || w.Body.Len() != 0 || r.calls != 1 || len(rec.evs) != 1 {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
	if rec.evs[0].Meta.UserAgent != "ua-test" || rec.evs[0].Meta.IP == "" {
		t.Fatalf("meta: %+v", rec.evs[0].Meta)
	}
	cs := w.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("cookies=%d", len(cs))
	}
	c := cs[0]
	if c.Name != middleware.SessionCookieName || c.Value != "" || c.MaxAge != -1 || c.Path != "/" ||
		!c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("%+v", c)
	}
	if got := w.Header().Get("Set-Cookie"); got == "" {
		t.Fatal("tanpa Set-Cookie")
	}
}

func TestHandler_InsecureConfig(t *testing.T) {
	h, _, _ := newH(1, nil, false)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req(true))
	if w.Result().Cookies()[0].Secure {
		t.Fatal("Secure harus false")
	}
}

func TestHandler_NoCookieStill204(t *testing.T) {
	h, r, _ := newH(1, nil, true)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req(false))
	if w.Code != 204 || r.calls != 0 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("code=%d calls=%d", w.Code, r.calls)
	}
}

func TestHandler_DBErrorKeepsCookie(t *testing.T) {
	h, _, _ := newH(0, errors.New("db down"), true)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req(true))
	if w.Code != 500 || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("code=%d cookie=%q", w.Code, w.Header().Get("Set-Cookie"))
	}
}
