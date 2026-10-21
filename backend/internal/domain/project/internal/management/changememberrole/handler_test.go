package changememberrole

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/lib/common/reqctx"
)

func serve(e env, body, projectID, userID, ua string) *httptest.ResponseRecorder {
	rt := chi.NewRouter()
	rt.Patch("/projects/{projectID}/members/{userID}", handler{uc: e.uc}.ServeHTTP)
	r := httptest.NewRequest(http.MethodPatch, "/projects/"+projectID+"/members/"+userID, strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:1234"
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	}
	ctx := reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: "u-1", Email: "a@tim.id", Role: "developer"})
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, r.WithContext(ctx))
	return w
}

func errOf(w *httptest.ResponseRecorder) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	e, _ := m["error"].(map[string]any)
	return e
}

func TestHandler_OK(t *testing.T) {
	e := newEnv("owner")
	w := serve(e, `{"role":"owner"}`, "p1", "u-2", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	m := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if m["user_id"] != "u-2" || m["role"] != "owner" || m["email"] != "b@tim.id" {
		t.Fatalf("body: %v", m)
	}
}

func TestHandler_BadBodies(t *testing.T) {
	for _, body := range []string{`{"rol":"owner"}`, `bukan json`, `{"role":5}`} {
		e := newEnv("owner")
		w := serve(e, body, "p1", "u-2", "")
		if w.Code != 400 || len(*e.log) != 0 {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestHandler_RoleRequired(t *testing.T) {
	for _, body := range []string{`{}`, `{"role":null}`, `{"role":""}`} {
		e := newEnv("owner")
		w := serve(e, body, "p1", "u-2", "")
		if w.Code != 400 || len(*e.log) != 0 {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
		d, _ := errOf(w)["details"].(map[string]any)
		f, _ := d["fields"].(map[string]any)
		if f["role"] != "wajib diisi" {
			t.Fatalf("%s: %s", body, w.Body.String())
		}
	}
}

func TestHandler_InvalidRoleForwarded(t *testing.T) {
	e := newEnv("owner")
	w := serve(e, `{"role":"admin"}`, "p1", "u-2", "")
	if w.Code != 400 || len(*e.log) == 0 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(e env)
		userID string
		status int
		code   string
	}{
		{"project 404", func(e env) { e.repo.found = false }, "u-2", 404, "NOT_FOUND"},
		{"member 404", func(e env) {}, "bukan-uuid", 404, "NOT_FOUND"},
		{"forbidden", func(e env) { e.repo.memberRole = "member" }, "u-2", 403, "FORBIDDEN"},
		{"last owner", func(e env) {}, "u-1", 409, "CONFLICT"},
	}
	for _, c := range cases {
		e := newEnv("owner")
		c.setup(e)
		role := "owner"
		if c.name == "last owner" {
			role = "member"
		}
		w := serve(e, `{"role":"`+role+`"}`, "p1", c.userID, "")
		if w.Code != c.status || errOf(w)["code"] != c.code {
			t.Fatalf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
		if c.name == "last owner" {
			d, _ := errOf(w)["details"].(map[string]any)
			if d["reason"] != "LAST_OWNER" {
				t.Fatalf("details: %s", w.Body.String())
			}
		}
	}
}

func TestHandler_InputForwarded(t *testing.T) {
	e := newEnv("owner")
	serve(e, `{"role":"owner"}`, "p1", "u-2", strings.Repeat("x", 600))
	if len(e.aud.events) != 1 {
		t.Fatal("audit")
	}
	m := e.aud.events[0].Meta
	if len(m.UserAgent) != 512 || m.IP == "" {
		t.Fatalf("meta: %+v", m)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("abc", 5) != "abc" || truncate("日本語日本語", 3) != "日本語" {
		t.Fatal("truncate")
	}
}
