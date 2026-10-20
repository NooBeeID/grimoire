package createuser

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/reqctx"
)

func serve(e env, body string) *httptest.ResponseRecorder {
	h := NewHandler(e.uc)
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-ua")
	req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "admin-1", Email: "admin@tim.id", Role: base.RoleSuperAdmin}))
	w := httptest.NewRecorder()
	h.Create(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, w.Body)
	}
	return m
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	return decode(t, w)["error"].(map[string]any)["code"].(string)
}

const okBody = `{"email":" Sari@Tim.ID ","name":"Sari","role":"developer"}`

func TestHandler_Created201(t *testing.T) {
	w := serve(setup(), okBody)
	if w.Code != 201 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("code=%d h=%v", w.Code, w.Header())
	}
	m := decode(t, w)
	u := m["user"].(map[string]any)
	if u["email"] != "sari@tim.id" || u["must_change_password"] != true || u["is_active"] != true || u["id"] != "new-id" {
		t.Fatalf("%v", u)
	}
	if tp, _ := m["temporary_password"].(string); len(tp) != 16 {
		t.Fatalf("temp=%v", m["temporary_password"])
	}
	if _, ok := u["password_hash"]; ok {
		t.Fatal("bocor hash")
	}
}

func TestHandler_TemporaryPasswordNullWhenProvided(t *testing.T) {
	w := serve(setup(), `{"email":"b@tim.id","name":"B","role":"developer","password":"awal-yang-panjang-1"}`)
	m := decode(t, w)
	v, ok := m["temporary_password"]
	if w.Code != 201 || !ok || v != nil {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "awal-yang-panjang-1") {
		t.Fatal("password bocor di respons")
	}
}

func TestHandler_DecodeErrors400(t *testing.T) {
	cases := map[string]string{
		"invalid json":          `{bad`,
		"wrong type":            `{"email":5}`,
		"empty body":            ``,
		"unknown is_active":     `{"email":"a@b.id","name":"A","role":"developer","is_active":false}`,
		"unknown must_change":   `{"email":"a@b.id","name":"A","role":"developer","must_change_password":false}`,
		"unknown password_hash": `{"email":"a@b.id","name":"A","role":"developer","password_hash":"x"}`,
		"unknown id":            `{"email":"a@b.id","name":"A","role":"developer","id":"x"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			e := setup()
			w := serve(e, body)
			if w.Code != 400 || errCode(t, w) != "INVALID_ARGUMENT" || e.r.calls != 0 {
				t.Fatalf("code=%d body=%s repo=%d", w.Code, w.Body, e.r.calls)
			}
		})
	}
}

func TestHandler_ValidationFields400(t *testing.T) {
	w := serve(setup(), `{"email":"x","name":"","role":"","password":"short"}`)
	if w.Code != 400 {
		t.Fatalf("code=%d", w.Code)
	}
	f := decode(t, w)["error"].(map[string]any)["details"].(map[string]any)["fields"].(map[string]any)
	if len(f) != 4 {
		t.Fatalf("%v", f)
	}
}

func TestHandler_ConflictMapsTo409(t *testing.T) {
	e := setup()
	e.r.err = base.ErrEmailTaken
	w := serve(e, okBody)
	if w.Code != 409 || errCode(t, w) != "CONFLICT" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestHandler_InternalErrorHidesDetail(t *testing.T) {
	e := setup()
	e.r.err = errors.New("pgx: connection refused SELECT secret")
	w := serve(e, `{"email":"b@tim.id","name":"B","role":"developer","password":"awal-yang-panjang-1"}`)
	body := w.Body.String()
	if w.Code != 500 || strings.Contains(body, "pgx") || strings.Contains(body, "SELECT") || strings.Contains(body, "awal-yang-panjang-1") {
		t.Fatalf("code=%d body=%s", w.Code, body)
	}
}

func TestHandler_SecretNotLoggedOnDecodeError(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(old)
	w := serve(setup(), `{"email":5,"password":"rahasia-banget-123"}`)
	if w.Code != 400 {
		t.Fatalf("code=%d", w.Code)
	}
	if strings.Contains(buf.String(), "rahasia-banget-123") || strings.Contains(w.Body.String(), "rahasia-banget-123") {
		t.Fatal("password bocor")
	}
}
