package middleware

import (
	"errors"
	"grimoire/internal/infra/http/respond"
	"net/http"
	"strings"
	"testing"

	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

func sessionChain(s *stack, secure bool) http.Handler {
	return Session(s.bus, secure)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := reqctx.IdentityFrom(r.Context())
		respond.WriteJSON(w, 200, map[string]any{"auth": ok, "user": id.UserID, "role": id.Role})
	}))
}

func TestSession_NoCookieOrEmptyCookieNoBusCall(t *testing.T) {
	s := newStack(t, nil, nil)
	n := registerSession(s.bus, func(string) (resolvesession.Response, error) { return resolvesession.Response{}, nil })
	h := sessionChain(s, false)
	if w := do(h, "GET", "/", nil, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"auth":false`) {
		t.Fatal(w.Body.String())
	}
	do(h, "GET", "/", map[string]string{"Cookie": SessionCookieName + "="}, "")
	if *n != 0 {
		t.Fatalf("bus dipanggil %d kali", *n)
	}
}

func TestSession_FoundSetsIdentityAndLogUser(t *testing.T) {
	s := newStack(t, nil, nil)
	var got string
	registerSession(s.bus, func(tok string) (resolvesession.Response, error) {
		got = tok
		return resolvesession.Response{Found: true, UserID: "u-7", Email: "a@b.c", Name: "A", Role: "developer"}, nil
	})
	h := TraceID()(AccessLog(s.log)(sessionChain(s, false)))
	w := do(h, "GET", "/", map[string]string{"Cookie": SessionCookieName + "=tok123"}, "")
	if got != "tok123" || !strings.Contains(w.Body.String(), `"user":"u-7"`) {
		t.Fatalf("%q %s", got, w.Body.String())
	}
	if m := s.logs.find(t, "http request"); m == nil || m["user_id"] != "u-7" {
		t.Fatalf("%v", m)
	}
}

func TestSession_NotFoundClearsCookie(t *testing.T) {
	for _, secure := range []bool{false, true} {
		s := newStack(t, nil, nil)
		registerSession(s.bus, func(string) (resolvesession.Response, error) { return resolvesession.Response{}, nil })
		w := do(sessionChain(s, secure), "GET", "/", map[string]string{"Cookie": SessionCookieName + "=stale"}, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"auth":false`) {
			t.Fatalf("harus lanjut anonim: %d %s", w.Code, w.Body.String())
		}
		c := w.Result().Cookies()
		if len(c) != 1 || c[0].Name != SessionCookieName || c[0].Value != "" || c[0].MaxAge != -1 ||
			!c[0].HttpOnly || c[0].SameSite != http.SameSiteLaxMode || c[0].Path != "/" || c[0].Secure != secure {
			t.Fatalf("%+v", c)
		}
		if !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
			t.Fatal(w.Header().Get("Set-Cookie"))
		}
	}
}

func TestSession_Errors500(t *testing.T) {
	cases := map[string]func(*stack){
		"bus error": func(s *stack) {
			registerSession(s.bus, func(string) (resolvesession.Response, error) {
				return resolvesession.Response{}, errors.New("db down host=10.1.1.1")
			})
		},
		"unregistered": func(s *stack) {},
		"found tanpa userid": func(s *stack) {
			registerSession(s.bus, func(string) (resolvesession.Response, error) {
				return resolvesession.Response{Found: true}, nil
			})
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			s := newStack(t, nil, nil)
			setup(s)
			w := do(sessionChain(s, false), "GET", "/", map[string]string{"Cookie": SessionCookieName + "=x"}, "")
			e := decodeErr(t, w)
			if w.Code != 500 || e.Error.Message != "terjadi kesalahan internal" {
				t.Fatalf("%d %+v", w.Code, e)
			}
			if strings.Contains(w.Body.String(), "10.1.1.1") || strings.Contains(w.Body.String(), "core.auth") {
				t.Fatal("bocor")
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("cookie tidak boleh dihapus")
			}
		})
	}
}

func TestSession_NilBus500(t *testing.T) {
	useLogger(t)
	h := Session(nil, false)(http.HandlerFunc(okHandler))
	if w := do(h, "GET", "/", map[string]string{"Cookie": SessionCookieName + "=x"}, ""); w.Code != 500 {
		t.Fatal(w.Code)
	}
}
