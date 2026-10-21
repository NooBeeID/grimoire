package updateproject

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/reqctx"
)

func serve(t *testing.T, e env, role, id, body, ua string) *httptest.ResponseRecorder {
	t.Helper()
	rt := chi.NewRouter()
	rt.Patch("/projects/{projectID}", handler{uc: e.uc}.ServeHTTP)
	r := httptest.NewRequest(http.MethodPatch, "/projects/"+id, strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:1234"
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	}
	ctx := reqctx.WithTraceID(r.Context(), "trace-1")
	if role != "" {
		ctx = reqctx.WithIdentity(ctx, reqctx.Identity{UserID: "u-1", Email: "a@tim.id", Role: role})
	}
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, r.WithContext(ctx))
	return w
}

func errBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("bukan JSON: %s", w.Body.String())
	}
	e, _ := m["error"].(map[string]any)
	return e
}

func TestHandler_OK(t *testing.T) {
	e := newEnv("member")
	w := serve(t, e, "developer", "p1", `{"name":"Baru"}`, "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	m := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	for _, k := range []string{"id", "name", "description", "created_at", "updated_at", "my_role", "access_level",
		"is_member", "member_count", "counts", "owner_count"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("kunci %s hilang: %s", k, w.Body.String())
		}
	}
	if m["name"] != "Baru" || m["access_level"] != "member" {
		t.Fatalf("body: %v", m)
	}
}

func TestHandler_BadBody400(t *testing.T) {
	for _, body := range []string{`{"nama":"x"}`, `bukan json`, `{"name":5}`} {
		e := newEnv("owner")
		w := serve(t, e, "developer", "p1", body, "")
		if w.Code != 400 || len(*e.log) != 0 {
			t.Fatalf("%s: %d log %v", body, w.Code, *e.log)
		}
	}
}

func TestHandler_EmptyAndNullBody400FromUsecase(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":null,"description":null}`} {
		e := newEnv("owner")
		w := serve(t, e, "developer", "p1", body, "")
		if w.Code != 400 {
			t.Fatalf("%s: %d", body, w.Code)
		}
		ee := errBody(t, w)
		fields := ee["details"].(map[string]any)["fields"].(map[string]any)
		if fields["body"] == nil {
			t.Fatalf("fields: %v", fields)
		}
		if len(*e.log) == 0 {
			t.Fatal("usecase harus dipanggil (otorisasi dulu)")
		}
	}
}

func TestParams_NullIsNil(t *testing.T) {
	var r UpdateRequest
	if err := json.Unmarshal([]byte(`{"name":null,"description":""}`), &r); err != nil {
		t.Fatal(err)
	}
	p := r.Patch()
	if p.Name != nil || p.Description == nil || *p.Description != "" {
		t.Fatalf("patch: %+v", p)
	}
}

func TestHandler_NotFoundForNonUUID(t *testing.T) {
	e := newEnv("owner")
	e.repo.found = false
	w := serve(t, e, "developer", "bukan-uuid", `{"name":"Baru"}`, "")
	if w.Code != 404 || errBody(t, w)["code"] != "NOT_FOUND" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_NonMember404(t *testing.T) {
	w := serve(t, newEnv(""), "developer", "p1", `{"name":"Baru"}`, "")
	if w.Code != 404 {
		t.Fatalf("%d", w.Code)
	}
}

func TestHandler_InternalError500(t *testing.T) {
	e := newEnv("owner")
	e.repo.failUpdate = http.ErrAbortHandler
	w := serve(t, e, "developer", "p1", `{"name":"Baru"}`, "")
	if w.Code != 500 || errBody(t, w)["code"] != "INTERNAL" || strings.Contains(w.Body.String(), "ErrAbortHandler") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_MetaTruncated(t *testing.T) {
	e := newEnv("owner")
	ua := strings.Repeat("é", 600)
	w := serve(t, e, "developer", "p1", `{"name":"Baru"}`, ua)
	if w.Code != 200 || len(e.aud.events) != 1 {
		t.Fatalf("%d", w.Code)
	}
	m := e.aud.events[0].Meta
	if m.IP == "" || len([]rune(m.UserAgent)) != 512 {
		t.Fatalf("meta: ip %q ua %d", m.IP, len([]rune(m.UserAgent)))
	}
	var _ base.RequestMeta = m
}
