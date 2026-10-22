package listkeys

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeAccess struct {
	calls   int
	project string
	actor   base.Actor
	err     error
}

func (f *fakeAccess) Require(_ context.Context, p string, a base.Actor) error {
	f.calls++
	f.project, f.actor = p, a
	return f.err
}

type fakeRepo struct {
	calls   int
	project string
	scope   base.OwnerScope
	keys    []base.Key
	err     error
}

func (f *fakeRepo) ListByProject(_ context.Context, p string, sc base.OwnerScope) ([]base.Key, error) {
	f.calls++
	f.project = p
	f.scope = sc
	return f.keys, f.err
}

func authCtx() context.Context { return authCtxAs("u1", "developer") }

func authCtxAs(id, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Role: role, Email: "a@tim.id"})
}

func TestExecute_NoIdentity(t *testing.T) {
	a, r := &fakeAccess{}, &fakeRepo{}
	_, err := NewUsecase(a, r).Execute(context.Background(), "p")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized || a.calls != 0 || r.calls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_AccessDeniedReturnsProjectNotFound(t *testing.T) {
	a, r := &fakeAccess{err: base.ErrProjectNotFound}, &fakeRepo{}
	_, err := NewUsecase(a, r).Execute(authCtx(), "p")
	if !errors.Is(err, base.ErrProjectNotFound) || r.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}

func TestExecute_AccessInfraErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	a, r := &fakeAccess{err: boom}, &fakeRepo{}
	_, err := NewUsecase(a, r).Execute(authCtx(), "p")
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) || r.calls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_AccessReceivesActorFromIdentity(t *testing.T) {
	a, r := &fakeAccess{}, &fakeRepo{}
	if _, err := NewUsecase(a, r).Execute(authCtx(), "proj-1"); err != nil {
		t.Fatal(err)
	}
	if a.project != "proj-1" || a.actor != (base.Actor{ID: "u1", Role: "developer", Label: "a@tim.id"}) {
		t.Fatalf("project=%q actor=%+v", a.project, a.actor)
	}
}

func TestT04_AC1_MemberAllowed(t *testing.T) {
	a, r := &fakeAccess{}, &fakeRepo{}
	if _, err := NewUsecase(a, r).Execute(authCtx(), "proj-1"); err != nil {
		t.Fatal(err)
	}
	if r.calls != 1 || r.project != "proj-1" {
		t.Fatalf("calls=%d project=%q", r.calls, r.project)
	}
}

func str(s string) *string { return &s }

func TestExecute_MapsEveryKeyToDTO(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	rev := now.Add(time.Hour)
	used := now.Add(30 * time.Minute)
	keys := []base.Key{
		{ID: "1", Name: "a", Prefix: "aaaaaaaa", CreatedAt: now, LastUsedAt: &used},
		{ID: "2", Name: "b", Prefix: "bbbbbbbb", CreatedAt: now},
		{ID: "3", Name: "c", Prefix: "cccccccc", CreatedAt: now, RevokedAt: &rev, RevokedBy: str("u9"), RevokedByLabel: str("x@tim.id")},
	}
	res, err := NewUsecase(&fakeAccess{}, &fakeRepo{keys: keys}).Execute(authCtx(), "p")
	if err != nil || len(res.Items) != 3 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	for i, id := range []string{"1", "2", "3"} {
		if res.Items[i].ID != id {
			t.Fatalf("order: %+v", res.Items)
		}
	}
	if res.Items[0].Status != base.StatusActive || res.Items[2].Status != base.StatusRevoked {
		t.Fatalf("status: %+v", res.Items)
	}
	if res.Items[0].LastUsedAt == nil || !res.Items[0].LastUsedAt.Equal(used) {
		t.Fatal("last_used_at")
	}
	c := res.Items[2]
	if c.RevokedAt == nil || !c.RevokedAt.Equal(rev) || *c.RevokedBy != "u9" || *c.RevokedByLabel != "x@tim.id" {
		t.Fatalf("revoked: %+v", c)
	}
}

func TestExecute_EmptyReturnsEmptySlice(t *testing.T) {
	for _, keys := range [][]base.Key{nil, {}} {
		res, err := NewUsecase(&fakeAccess{}, &fakeRepo{keys: keys}).Execute(authCtx(), "p")
		if err != nil || res.Items == nil || len(res.Items) != 0 {
			t.Fatalf("res=%+v err=%v", res, err)
		}
		b, _ := json.Marshal(res)
		if string(b) != `{"items":[]}` {
			t.Fatalf("json=%s", b)
		}
	}
}

func TestExecute_RepoErrorWrapped(t *testing.T) {
	boom := errors.New("db down")
	res, err := NewUsecase(&fakeAccess{}, &fakeRepo{err: boom}).Execute(authCtx(), "p")
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) || res.Items != nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestExecute_DTOHasNoSecretFields(t *testing.T) {
	hash := make([]byte, 32)
	for i := range hash {
		hash[i] = byte(i + 1)
	}
	keys := []base.Key{{ID: "1", Name: "a", Prefix: "aaaaaaaa", SecretHash: hash, CreatedAt: time.Now()}}
	res, _ := NewUsecase(&fakeAccess{}, &fakeRepo{keys: keys}).Execute(authCtx(), "p")
	b, _ := json.Marshal(res)
	s := string(b)
	for _, bad := range []string{"secret", "hash", "raw_key", "0102030405"} {
		if strings.Contains(s, bad) {
			t.Fatalf("leak %q in %s", bad, s)
		}
	}
}

func TestExecute_ScopeFromRole(t *testing.T) {
	cases := []struct {
		name, id, role string
		want           base.OwnerScope
	}{
		{"developer", "u-a", "developer", base.OwnerScope{OwnerID: "u-a"}},
		{"super_admin", "u-s", "super_admin", base.OwnerScope{All: true}},
		{"role kosong", "u-x", "", base.OwnerScope{OwnerID: "u-x"}},
		{"role tak dikenal", "u-y", "hacker", base.OwnerScope{OwnerID: "u-y"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeRepo{}
			if _, err := NewUsecase(&fakeAccess{}, r).Execute(authCtxAs(c.id, c.role), "p"); err != nil {
				t.Fatal(err)
			}
			if r.calls != 1 || r.scope != c.want {
				t.Fatalf("calls=%d scope=%+v want %+v", r.calls, r.scope, c.want)
			}
		})
	}
}

type orderRecorder struct{ log *[]string }

func (o orderRecorder) Require(context.Context, string, base.Actor) error {
	*o.log = append(*o.log, "access")
	return nil
}

type orderRepo struct{ log *[]string }

func (o orderRepo) ListByProject(context.Context, string, base.OwnerScope) ([]base.Key, error) {
	*o.log = append(*o.log, "repo")
	return nil, nil
}

func TestExecute_AccessBeforeRepo(t *testing.T) {
	var log []string
	if _, err := NewUsecase(orderRecorder{&log}, orderRepo{&log}).Execute(authCtx(), "p"); err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 || log[0] != "access" || log[1] != "repo" {
		t.Fatalf("log=%v", log)
	}
}

func TestExecute_NoInMemoryFiltering(t *testing.T) {
	keys := []base.Key{
		{ID: "1", Name: "own", Prefix: "aaaaaaaa", CreatedBy: str("u1"), CreatedAt: time.Now()},
		{ID: "2", Name: "other", Prefix: "bbbbbbbb", CreatedBy: str("u2"), CreatedAt: time.Now()},
		{ID: "3", Name: "legacy", Prefix: "cccccccc", CreatedAt: time.Now()},
	}
	res, err := NewUsecase(&fakeAccess{}, &fakeRepo{keys: keys}).Execute(authCtx(), "p")
	if err != nil || len(res.Items) != 3 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
