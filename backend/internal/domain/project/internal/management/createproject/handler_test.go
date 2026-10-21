package createproject

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/reqctx"
)

func serve(t *testing.T, e env, role, body, ua string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:1234"
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	}
	ctx := r.Context()
	if role != "" {
		ctx = reqctx.WithIdentity(ctx, reqctx.Identity{UserID: "u-1", Email: "a@tim.id", Role: role})
	}
	ctx = reqctx.WithTraceID(ctx, "trace-1")
	w := httptest.NewRecorder()
	newHandler(e.uc).ServeHTTP(w, r.WithContext(ctx))
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("bukan JSON: %s", w.Body.String())
	}
	return m
}

func errOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := decodeBody(t, w)
	if m["trace_id"] == nil || m["trace_id"] == "" {
		t.Fatalf("trace_id hilang: %s", w.Body.String())
	}
	e, _ := m["error"].(map[string]any)
	return e
}

func TestHandler_Created_Developer(t *testing.T) {
	w := serve(t, newEnv(), "developer", `{"name":"Payments","description":"d"}`, "")
	if w.Code != 201 {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
	m := decodeBody(t, w)
	for _, k := range []string{"id", "name", "description", "created_by", "created_by_label", "created_at", "updated_at",
		"my_role", "access_level", "is_member", "member_count", "counts"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("kunci %s hilang: %s", k, w.Body.String())
		}
	}
	if len(m) != 12 {
		t.Fatalf("kunci berlebih: %s", w.Body.String())
	}
	if m["my_role"] != "owner" || m["access_level"] != "owner" || m["created_at"] != "2026-10-22T09:00:00Z" {
		t.Fatalf("body: %s", w.Body.String())
	}
	c := m["counts"].(map[string]any)
	if c["files"] != float64(0) || c["specs"] != float64(0) || c["keys"] != float64(0) {
		t.Fatalf("counts: %v", c)
	}
}

func TestHandler_Created_SuperAdmin(t *testing.T) {
	w := serve(t, newEnv(), "super_admin", `{"name":"Payments"}`, "")
	m := decodeBody(t, w)
	if w.Code != 201 || m["my_role"] != nil || m["access_level"] != "admin" || m["is_member"] != false || m["member_count"] != float64(0) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_BadBody(t *testing.T) {
	for name, body := range map[string]string{
		"unknown": `{"nama":"x"}`, "empty": ``, "broken": `{"name":`, "number": `{"name":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv()
			w := serve(t, e, "developer", body, "")
			if w.Code != 400 || errOf(t, w)["code"] != "INVALID_ARGUMENT" || e.repo.inTx != 0 {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandler_ValidationDetails(t *testing.T) {
	w := serve(t, newEnv(), "developer", `{"description":"x"}`, "")
	if w.Code != 400 {
		t.Fatalf("code %d", w.Code)
	}
	d := errOf(t, w)["details"].(map[string]any)["fields"].(map[string]any)
	if _, ok := d["name"]; !ok {
		t.Fatalf("details: %v", d)
	}
}

func TestHandler_Forbidden_Unauthorized(t *testing.T) {
	w := serve(t, newEnv(), "agent", `{"name":"Payments"}`, "")
	if w.Code != 403 || errOf(t, w)["code"] != "FORBIDDEN" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = serve(t, newEnv(), "", `{"name":"Payments"}`, "")
	if w.Code != 401 || errOf(t, w)["code"] != "UNAUTHORIZED" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_InternalErrorIsGeneric(t *testing.T) {
	e := newEnv()
	e.repo.failProj = errors.New("pq: secret detail")
	w := serve(t, e, "developer", `{"name":"Payments"}`, "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") || errOf(t, w)["code"] != "INTERNAL" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_MetaTruncatesUserAgent(t *testing.T) {
	e := newEnv()
	ua := strings.Repeat("é", 600)
	if w := serve(t, e, "developer", `{"name":"Payments"}`, ua); w.Code != 201 {
		t.Fatalf("code %d", w.Code)
	}
	m := e.aud.events[0].Meta
	if utf8.RuneCountInString(m.UserAgent) != 512 || m.IP != "10.0.0.5" {
		t.Fatalf("meta: ip=%q ua runes=%d", m.IP, utf8.RuneCountInString(m.UserAgent))
	}
}

func TestTruncate(t *testing.T) {
	if truncate("abc", 5) != "abc" || truncate("abcdef", 3) != "abc" || truncate("ééé", 2) != "éé" {
		t.Fatal("truncate")
	}
}

var _ = context.Background
var _ base.Clock
