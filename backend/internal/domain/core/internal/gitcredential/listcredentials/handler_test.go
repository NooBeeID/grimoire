package listcredentials

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/gitcredential/base"
)

func serve(r *fakeRepo, withID bool, target string, s Settings) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, strings.NewReader("acak"))
	if withID {
		req = req.WithContext(authCtxAs("u1", "developer"))
	}
	rec := httptest.NewRecorder()
	newHandler(NewUsecase(r, s)).ServeHTTP(rec, req)
	return rec
}

func TestHandler_OKShape(t *testing.T) {
	r := &fakeRepo{creds: []base.Credential{{Provider: "github", Host: "github.com", Last4: "7Qx2", TokenEnc: []byte("CANARY"), CreatedAt: t0, UpdatedAt: t0}}}
	rec := serve(r, true, "/me/git-credentials?user_id=lain", Settings{Enabled: true, GitLabHosts: []string{"gitlab.com"}})
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("code=%d h=%v", rec.Code, rec.Header())
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &top); err != nil || len(top) != 3 {
		t.Fatalf("top=%v err=%v", top, err)
	}
	var items []map[string]any
	_ = json.Unmarshal(top["items"], &items)
	if len(items) != 1 || len(items[0]) != 5 {
		t.Fatalf("items=%v", items)
	}
	for _, k := range []string{"provider", "host", "last4", "created_at", "updated_at"} {
		if _, ok := items[0][k]; !ok {
			t.Fatalf("missing %s", k)
		}
	}
	low := strings.ToLower(rec.Body.String())
	if strings.Contains(low, "token") || strings.Contains(low, "canary") {
		t.Fatalf("leak: %s", rec.Body.String())
	}
	if r.userID != "u1" {
		t.Fatalf("userID=%q", r.userID)
	}
}

func TestHandler_Disabled200(t *testing.T) {
	rec := serve(&fakeRepo{}, true, "/me/git-credentials", Settings{})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"enabled":false`) || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_Unauthorized(t *testing.T) {
	rec := serve(&fakeRepo{}, false, "/me/git-credentials", Settings{})
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), `"code":"UNAUTHORIZED"`) || rec.Header().Get("Cache-Control") == "no-store" {
		t.Fatalf("code=%d body=%s h=%v", rec.Code, rec.Body.String(), rec.Header())
	}
}

func TestHandler_InternalError(t *testing.T) {
	rec := serve(&fakeRepo{err: errors.New("connection refused: dsn=postgres://secret")}, true, "/me/git-credentials", Settings{})
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"code":"INTERNAL"`) || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}
