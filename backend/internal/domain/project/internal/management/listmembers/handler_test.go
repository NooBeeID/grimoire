package listmembers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
)

type fakeExec struct {
	res Result
	err error
	got string
}

func (f *fakeExec) Execute(_ context.Context, id string) (Result, error) {
	f.got = id
	return f.res, f.err
}

func serve(f *fakeExec, target, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/projects/{projectID}/members", newHandler(f).ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_OK(t *testing.T) {
	name, email, by := "Andi", "a@x.id", "u1"
	f := &fakeExec{res: Result{Items: []base.MemberDTO{
		{UserID: "u1", Name: &name, Email: &email, IsActive: true, Found: true, Role: "owner",
			AddedAt: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC), AddedBy: &by, AddedByLabel: &email},
		{UserID: "u2", Role: "member", AddedAt: time.Date(2026, 10, 22, 9, 5, 0, 0, time.UTC)},
	}}}
	w := serve(f, "/projects/p1/members?x=1", `{"junk":true}`)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || f.got != "p1" {
		t.Fatalf("%d %v %q", w.Code, w.Header(), f.got)
	}
	var m struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || len(m.Items) != 2 {
		t.Fatalf("%v %s", err, w.Body)
	}
	for _, k := range []string{"user_id", "name", "email", "is_active", "found", "role", "added_at", "added_by", "added_by_label"} {
		if _, ok := m.Items[0][k]; !ok {
			t.Fatalf("missing %s", k)
		}
	}
	if m.Items[0]["added_at"] != "2026-10-22T09:00:00Z" {
		t.Fatalf("%v", m.Items[0]["added_at"])
	}
	u2 := m.Items[1]
	if u2["name"] != nil || u2["email"] != nil || u2["found"] != false {
		t.Fatalf("%v", u2)
	}
}

func TestHandler_EmptyItemsIsArray(t *testing.T) {
	for _, res := range []Result{{}, {Items: []base.MemberDTO{}}} {
		w := serve(&fakeExec{res: res}, "/projects/p1/members", "")
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"items":[]}` {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
}

func TestHandler_NotFound(t *testing.T) {
	w := serve(&fakeExec{err: base.ErrProjectNotFound}, "/projects/p1/members", "")
	if w.Code != 404 || !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandler_InternalErrorNotLeaked(t *testing.T) {
	w := serve(&fakeExec{err: errors.New("secret-detail")}, "/projects/p1/members", "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret-detail") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
