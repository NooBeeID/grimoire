package getuser

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
)

func serve(repo *fakeRepo, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/api/users/{id}", NewHandler(NewUsecase(repo, fixedClock)).Get)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func TestHandler_Get_200(t *testing.T) {
	u := sampleUser()
	w := serve(&fakeRepo{user: u, found: true}, "/api/users/"+u.ID)
	if w.Code != 200 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("ct=%q", ct)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("Cache-Control")
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["id"] != u.ID || m["email"] != "sari@tim.id" || m["role"] != "developer" ||
		m["is_active"] != true || m["last_login_at"] != nil {
		t.Fatalf("body=%v", m)
	}
	if _, ok := m["last_login_at"]; !ok {
		t.Fatal("kunci last_login_at harus ada")
	}
	if _, ok := m["password_hash"]; ok {
		t.Fatal("bocor hash")
	}
}

func TestHandler_Get_404_NotFound(t *testing.T) {
	w := serve(&fakeRepo{}, "/api/users/abc")
	if w.Code != 404 {
		t.Fatalf("code=%d", w.Code)
	}
	var m struct {
		Error   struct{ Code, Message string }
		TraceID string `json:"trace_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if m.Error.Code != "NOT_FOUND" || m.Error.Message != base.ErrUserNotFound.Message {
		t.Fatalf("body=%s", w.Body)
	}
}

func TestHandler_Get_PassesRawIDToUsecase(t *testing.T) {
	repo := &fakeRepo{}
	w := serve(repo, "/api/users/bukan-uuid")
	if w.Code != 404 || repo.gotID != "bukan-uuid" {
		t.Fatalf("code=%d id=%q", w.Code, repo.gotID)
	}
}

func TestHandler_Get_IgnoresQuery(t *testing.T) {
	u := sampleUser()
	w := serve(&fakeRepo{user: u, found: true}, "/api/users/"+u.ID+"?x=1")
	if w.Code != 200 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestHandler_Get_500_Generic(t *testing.T) {
	w := serve(&fakeRepo{err: errors.New("secret sql detail")}, "/api/users/x")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret sql detail") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}
