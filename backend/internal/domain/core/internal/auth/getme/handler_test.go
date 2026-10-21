package getme

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/internal/infra/mailer/mailertest"
	"grimoire/lib/common/reqctx"
)

func serve(r *http.Request) *httptest.ResponseRecorder {
	return serveWith(&mailertest.Fake{}, r)
}

func serveWith(m *mailertest.Fake, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	NewHandler(NewUsecase(m)).Get(w, r)
	return w
}

func withID(r *http.Request) *http.Request {
	return r.WithContext(reqctx.WithIdentity(r.Context(), reqctx.Identity{
		UserID: "u1", Email: "budi@tim.id", Name: "Budi", Role: "developer", MustChangePassword: true,
	}))
}

func TestHandler_Get_200(t *testing.T) {
	w := serve(withID(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)))
	if w.Code != 200 {
		t.Fatalf("code=%d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("ct=%q", ct)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("tanpa no-store")
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"id": "u1", "email": "budi@tim.id", "name": "Budi", "role": "developer", "must_change_password": true, "email_enabled": true}
	if len(got) != len(want) {
		t.Fatalf("keys: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s=%v", k, got[k])
		}
	}
}

func TestHandler_Get_IgnoresQueryAndBody(t *testing.T) {
	w := serve(withID(httptest.NewRequest(http.MethodGet, "/api/auth/me?x=1", strings.NewReader("{{acak"))))
	if w.Code != 200 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestHandler_Get_NoIdentity401(t *testing.T) {
	w := serve(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if w.Code != 401 || w.Header().Get("Cache-Control") == "no-store" {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestHandler_Get_EmailEnabledFalseWhenNotConfigured(t *testing.T) {
	w := serveWith(&mailertest.Fake{NotConfigured: true}, withID(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)))
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || got["email_enabled"] != false || got["id"] != "u1" || got["must_change_password"] != true {
		t.Fatalf("code=%d got=%v", w.Code, got)
	}
}
