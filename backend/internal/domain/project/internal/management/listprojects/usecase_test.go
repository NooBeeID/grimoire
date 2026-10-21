package listprojects

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeRepo struct {
	items    []Item
	total    int
	err      error
	calls    int
	actorID  string
	restrict bool
	filter   ProjectFilter
}

func (f *fakeRepo) List(_ context.Context, actorID string, restrict bool, flt ProjectFilter) ([]Item, int, error) {
	f.calls++
	f.actorID, f.restrict, f.filter = actorID, restrict, flt
	return f.items, f.total, f.err
}

type fakeCounter struct {
	calls [][]string
	out   map[string]base.ContentCounts
}

func (f *fakeCounter) ForProjects(_ context.Context, ids []string) map[string]base.ContentCounts {
	f.calls = append(f.calls, append([]string(nil), ids...))
	if f.out == nil {
		return map[string]base.ContentCounts{}
	}
	return f.out
}

func ctxAs(role, id string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Role: role, Email: id + "@x"})
}

func item(id, myRole string) Item {
	return Item{Project: base.Project{ID: id, Name: "P" + id}, MyRole: myRole, MemberCount: 2}
}

func TestUsecase_InvalidFilterIs400NoRepoNoCounts(t *testing.T) {
	r, c := &fakeRepo{}, &fakeCounter{}
	_, err := NewUsecase(r, c).Execute(ctxAs("developer", "a"), Input{Limit: ip(500)})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("%v", err)
	}
	d, _ := e.Details.(map[string]any)
	if fe, _ := d["fields"].(base.FieldErrors); fe["limit"] == "" {
		t.Fatalf("%+v", e.Details)
	}
	if r.calls != 0 || len(c.calls) != 0 {
		t.Fatal("repo/counter terpanggil")
	}
}

func TestUsecase_DeveloperUsesRestrictedQuery(t *testing.T) {
	r := &fakeRepo{}
	uc := NewUsecase(r, &fakeCounter{})
	if _, err := uc.Execute(ctxAs("developer", "dev1"), Input{}); err != nil || !r.restrict || r.actorID != "dev1" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := uc.Execute(ctxAs("super_admin", "sa1"), Input{}); err != nil || r.restrict || r.actorID != "sa1" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestUsecase_LevelAndMembership(t *testing.T) {
	r := &fakeRepo{items: []Item{item("1", "owner"), item("2", "member")}, total: 2}
	out, err := NewUsecase(r, &fakeCounter{}).Execute(ctxAs("developer", "a"), Input{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Items[0].AccessLevel != base.LevelOwner || !out.Items[0].IsMember ||
		out.Items[1].AccessLevel != base.LevelMember || !out.Items[1].IsMember {
		t.Fatalf("%+v", out.Items)
	}
	r = &fakeRepo{items: []Item{item("1", ""), item("2", "member")}, total: 2}
	out, _ = NewUsecase(r, &fakeCounter{}).Execute(ctxAs("super_admin", "s"), Input{})
	if out.Items[0].AccessLevel != base.LevelAdmin || out.Items[0].MyRole != nil || out.Items[0].IsMember {
		t.Fatalf("%+v", out.Items[0])
	}
	if out.Items[1].AccessLevel != base.LevelAdmin || !out.Items[1].IsMember || *out.Items[1].MyRole != "member" {
		t.Fatalf("%+v", out.Items[1])
	}
}

func TestUsecase_CountsBatchedOnce(t *testing.T) {
	var items []Item
	var want []string
	for i := 0; i < 20; i++ {
		id := string(rune('a' + i))
		items = append(items, item(id, "member"))
		want = append(want, id)
	}
	c := &fakeCounter{}
	if _, err := NewUsecase(&fakeRepo{items: items, total: 20}, c).Execute(ctxAs("developer", "a"), Input{}); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 1 || strings.Join(c.calls[0], ",") != strings.Join(want, ",") {
		t.Fatalf("%v", c.calls)
	}
	c = &fakeCounter{}
	if out, err := NewUsecase(&fakeRepo{}, c).Execute(ctxAs("developer", "a"), Input{}); err != nil || len(out.Items) != 0 {
		t.Fatal(err)
	}
	if len(c.calls) != 1 || len(c.calls[0]) != 0 {
		t.Fatalf("%v", c.calls)
	}
}

func TestUsecase_CountsMissingIsNull(t *testing.T) {
	three := 3
	c := &fakeCounter{out: map[string]base.ContentCounts{"1": {Files: &three}}}
	out, _ := NewUsecase(&fakeRepo{items: []Item{item("1", "owner"), item("2", "owner")}}, c).Execute(ctxAs("developer", "a"), Input{})
	b, _ := json.Marshal(out)
	s := string(b)
	if !strings.Contains(s, `"counts":{"files":3,"specs":null,"keys":null}`) ||
		!strings.Contains(s, `"counts":{"files":null,"specs":null,"keys":null}`) {
		t.Fatal(s)
	}
}

func TestUsecase_EmptyItemsIsNotNil(t *testing.T) {
	out, err := NewUsecase(&fakeRepo{}, &fakeCounter{}).Execute(ctxAs("developer", "a"), Input{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), `"items":[]`) {
		t.Fatal(string(b))
	}
}

func TestUsecase_EffectiveLimitOffsetEchoed(t *testing.T) {
	out, _ := NewUsecase(&fakeRepo{}, &fakeCounter{}).Execute(ctxAs("developer", "a"), Input{})
	if out.Limit != 20 || out.Offset != 0 {
		t.Fatalf("%+v", out)
	}
	out, _ = NewUsecase(&fakeRepo{}, &fakeCounter{}).Execute(ctxAs("developer", "a"), Input{Limit: ip(5), Offset: ip(10)})
	if out.Limit != 5 || out.Offset != 10 {
		t.Fatalf("%+v", out)
	}
}

func TestUsecase_UnknownRoleForbidden(t *testing.T) {
	r := &fakeRepo{}
	_, err := NewUsecase(r, &fakeCounter{}).Execute(ctxAs("agent", "a"), Input{})
	if !errors.Is(err, base.ErrForbidden) || r.calls != 0 {
		t.Fatalf("%v", err)
	}
}

func TestUsecase_NoIdentityUnauthorized(t *testing.T) {
	_, err := NewUsecase(&fakeRepo{}, &fakeCounter{}).Execute(context.Background(), Input{})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatalf("%v", err)
	}
}

func TestUsecase_RepoErrorWrapped500(t *testing.T) {
	r := &fakeRepo{err: errors.New("pq: syntax error SELECT secret")}
	_, err := NewUsecase(r, &fakeCounter{}).Execute(ctxAs("developer", "a"), Input{})
	var e *errs.Error
	if err == nil || errors.As(err, &e) || !strings.HasPrefix(err.Error(), "listprojects:") {
		t.Fatalf("%v", err)
	}
	w := httptest.NewRecorder()
	respond.WriteError(w, httptest.NewRequest("GET", "/projects", nil), err)
	if w.Code != 500 || strings.Contains(w.Body.String(), "SELECT") || !strings.Contains(w.Body.String(), "INTERNAL") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHandler_Headers(t *testing.T) {
	h := newHandler(NewUsecase(&fakeRepo{items: []Item{item("1", "owner")}, total: 1}, &fakeCounter{}))
	req := httptest.NewRequest("GET", "/projects", nil).WithContext(ctxAs("developer", "a"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
}

func TestHandler_BadShapeAnd400(t *testing.T) {
	r := &fakeRepo{}
	h := newHandler(NewUsecase(r, &fakeCounter{}))
	for _, q := range []string{"?limit=abc", "?limit=500&offset=-1"} {
		req := httptest.NewRequest("GET", "/projects"+q, nil).WithContext(ctxAs("developer", "a"))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 400 || !strings.Contains(w.Body.String(), `"fields"`) {
			t.Fatalf("%s: %d %s", q, w.Code, w.Body)
		}
	}
	if r.calls != 0 {
		t.Fatal("repo terpanggil")
	}
}
