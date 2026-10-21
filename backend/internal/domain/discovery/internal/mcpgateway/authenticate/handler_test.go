package authenticate

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/project/apikey/verify"
)

type failReader struct{ t *testing.T }

func (f failReader) Read([]byte) (int, error) {
	f.t.Error("body request tidak boleh dibaca")
	return 0, errors.New("read")
}

func do(t *testing.T, e *env, next http.Handler, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/mcp", failReader{t})
	r.RemoteAddr = "10.0.0.7:1234"
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	newHandler(e.uc, nil).middleware(next).ServeHTTP(w, r)
	return w
}

var neverNext = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("next dipanggil") })

func TestT07_AC2_IdenticalBody401(t *testing.T) {
	e := newEnv(t, 100, 10, true)
	e.verifyFn = func(r verify.Request) (verify.Response, error) {
		if r.RawKey == "revoked" {
			return verify.Response{Reason: verify.ReasonRevoked, Prefix: "abcd1234"}, nil
		}
		return verify.Response{Reason: verify.ReasonUnknown, Prefix: "abcd1234"}, nil
	}
	var bodies [][]byte
	for _, h := range []map[string]string{nil, {"X-API-Key": "random"}, {"X-API-Key": "revoked"}, {"Authorization": "Basic xx"}} {
		w := do(t, e, neverNext, h)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") != `Bearer realm="grimoire-mcp"` ||
			w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%d %v", w.Code, w.Header())
		}
		bodies = append(bodies, w.Body.Bytes())
		all := w.Body.String() + w.Header().Get("WWW-Authenticate")
		for _, bad := range []string{"unknown", "revoked", "missing", "malformed", "mismatch", "abcd1234"} {
			if strings.Contains(all, bad) {
				t.Fatalf("bocor %q", bad)
			}
		}
	}
	want := `{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}`
	for _, b := range bodies {
		if !bytes.Equal(b, bodies[0]) || string(b) != want {
			t.Fatalf("%s", b)
		}
	}
}

func TestT07_Handler429Key(t *testing.T) {
	e := newEnv(t, 100, 1, true)
	h := map[string]string{"Authorization": "Bearer " + keyA}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	do(t, e, ok, h)
	w := do(t, e, neverNext, h)
	ra, err := strconv.Atoi(w.Header().Get("Retry-After"))
	if w.Code != 429 || err != nil || ra < 1 {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	want := `{"jsonrpc":"2.0","id":null,"error":{"code":-32003,"message":"Rate limit exceeded","data":{"retry_after_seconds":` + strconv.Itoa(ra) + `}}}`
	if w.Body.String() != want || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%s", w.Body.String())
	}
}

func TestT07_Handler429IP(t *testing.T) {
	e := newEnv(t, 1, 10, true)
	do(t, e, neverNext, nil) // gagal, konsumsi token
	w := do(t, e, neverNext, map[string]string{"X-API-Key": keyA})
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), "-32003") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestT07_Handler503(t *testing.T) {
	e := newEnv(t, 10, 10, true)
	e.verifyFn = func(verify.Request) (verify.Response, error) { return verify.Response{}, errors.New("down") }
	w := do(t, e, neverNext, map[string]string{"X-API-Key": keyA})
	if w.Code != 503 || w.Header().Get("WWW-Authenticate") != "" ||
		w.Body.String() != `{"jsonrpc":"2.0","id":null,"error":{"code":-32004,"message":"Service unavailable"}}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestT07_HandlerAdmitted(t *testing.T) {
	e := newEnv(t, 10, 10, true)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		p, ok := base.PrincipalFrom(r.Context())
		if !ok || p.KeyID != "k1" || p.ProjectName != "Payments" || reqctx.APIKeyID(r.Context()) != "k1" {
			t.Errorf("principal %+v ok=%v", p, ok)
		}
	})
	w := do(t, e, next, map[string]string{"Authorization": "Bearer " + keyA})
	if !called || w.Code != 200 {
		t.Fatalf("called=%v code=%d", called, w.Code)
	}
}
