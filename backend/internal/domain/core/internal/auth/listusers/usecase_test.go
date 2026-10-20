package listusers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/errs"
)

type fakeRepo struct {
	users []base.User
	total int
	err   error
	calls int
	got   base.UserFilter
}

func (f *fakeRepo) List(_ context.Context, flt base.UserFilter) ([]base.User, int, error) {
	f.calls++
	f.got = flt
	return f.users, f.total, f.err
}

func TestParseQuery_Shape(t *testing.T) {
	_, fe := parseQuery(url.Values{"limit": {"abc"}, "offset": {"1.5"}})
	if fe["limit"] == "" || fe["offset"] == "" {
		t.Fatalf("fe=%v", fe)
	}
	in, fe := parseQuery(url.Values{"limit": {""}, "foo": {"bar"}})
	if len(fe) != 0 || in.Limit != nil || in.Offset != nil {
		t.Fatalf("in=%+v fe=%v", in, fe)
	}
	in, fe = parseQuery(url.Values{"limit": {"20"}, "role": {"developer"}, "q": {" x "}})
	if len(fe) != 0 || in.Limit == nil || *in.Limit != 20 || in.Role != "developer" || in.Query != " x " {
		t.Fatalf("in=%+v fe=%v", in, fe)
	}
}

func TestUsecase_InvalidFilterIs400NoRepoCall(t *testing.T) {
	r := &fakeRepo{}
	_, err := NewUsecase(r).Execute(context.Background(), Input{Status: "gone"})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid || r.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
	m, _ := e.Details.(map[string]any)
	fields, _ := m["fields"].(base.FieldErrors)
	if fields["status"] == "" {
		t.Fatalf("details=%v", e.Details)
	}
}

func TestUsecase_MapsToDTOWithoutHash(t *testing.T) {
	l := time.Date(2026, 10, 21, 9, 10, 0, 0, time.UTC)
	r := &fakeRepo{users: []base.User{{ID: "1", Email: "a@x.id", Name: "A", PasswordHash: "$argon2id$secret",
		Role: base.RoleDeveloper, LastLoginAt: &l}}, total: 1}
	out, err := NewUsecase(r).Execute(context.Background(), Input{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	s := string(b)
	if strings.Contains(s, "password_hash") || strings.Contains(s, "argon2id") || !strings.Contains(s, `"last_login_at":"2026-10-21T09:10:00Z"`) {
		t.Fatalf("json=%s", s)
	}
}

func TestUsecase_EmptyItemsIsNotNil(t *testing.T) {
	out, err := NewUsecase(&fakeRepo{}).Execute(context.Background(), Input{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), `"items":[]`) {
		t.Fatalf("json=%s", b)
	}
}

func TestUsecase_EffectiveLimitOffsetEchoed(t *testing.T) {
	out, err := NewUsecase(&fakeRepo{}).Execute(context.Background(), Input{})
	if err != nil || out.Limit != 20 || out.Offset != 0 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestUsecase_RepoErrorWrapped500(t *testing.T) {
	_, err := NewUsecase(&fakeRepo{err: errors.New("pq: syntax error SELECT")}).Execute(context.Background(), Input{})
	var e *errs.Error
	if err == nil || errors.As(err, &e) || !strings.HasPrefix(err.Error(), "listusers:") {
		t.Fatalf("err=%v", err)
	}
	h := NewHandler(NewUsecase(&fakeRepo{err: errors.New("pq: syntax error SELECT")}))
	w := httptest.NewRecorder()
	h.List(w, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "SELECT") || !strings.Contains(w.Body.String(), "INTERNAL") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestHandler_Headers(t *testing.T) {
	h := NewHandler(NewUsecase(&fakeRepo{}))
	w := httptest.NewRecorder()
	h.List(w, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("code=%d hdr=%v", w.Code, w.Header())
	}
}

func TestHandler_ShapeError400(t *testing.T) {
	r := &fakeRepo{}
	h := NewHandler(NewUsecase(r))
	w := httptest.NewRecorder()
	h.List(w, httptest.NewRequest(http.MethodGet, "/api/users?limit=abc", nil))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "harus berupa angka bulat") || r.calls != 0 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
}

func TestWhere_Parameterized(t *testing.T) {
	act := true
	clause, args := where(base.UserFilter{Role: "developer", IsActive: &act, Query: "x'; DROP"})
	if strings.Contains(clause, "DROP") || len(args) != 3 || !strings.Contains(clause, "$3 ESCAPE '\\' OR name ILIKE $3") {
		t.Fatalf("clause=%s args=%v", clause, args)
	}
}
