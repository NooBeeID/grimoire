package getproject

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeRepo struct {
	p           base.Project
	role        string
	found       bool
	findErr     error
	members     int
	owners      int
	statsErr    error
	statsCalls  int
	findCalls   int
	lastProject string
	lastUser    string
}

func (f *fakeRepo) FindAccess(_ context.Context, pid, uid string) (base.Project, string, bool, error) {
	f.findCalls++
	f.lastProject, f.lastUser = pid, uid
	return f.p, f.role, f.found, f.findErr
}

func (f *fakeRepo) MemberStats(context.Context, string) (int, int, error) {
	f.statsCalls++
	return f.members, f.owners, f.statsErr
}

type fakeCounter struct {
	out   map[string]base.ContentCounts
	calls int
	ids   []string
}

func (f *fakeCounter) ForProjects(_ context.Context, ids []string) map[string]base.ContentCounts {
	f.calls++
	f.ids = ids
	return f.out
}

func ctxFor(role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: role, Email: "u@x.id"})
}

func newRepoFound(role string) *fakeRepo {
	return &fakeRepo{p: base.Project{ID: "p1", Name: "P"}, role: role, found: true, members: 2, owners: 1}
}

func TestUsecase_Execute_OwnerSees(t *testing.T) {
	r, c := newRepoFound("owner"), &fakeCounter{}
	d, err := NewUsecase(r, c).Execute(ctxFor("developer"), "p1")
	if err != nil || d.AccessLevel != base.LevelOwner || d.MyRole == nil || *d.MyRole != "owner" || !d.IsMember ||
		d.MemberCount != 2 || d.OwnerCount != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	if r.lastUser != "u1" || r.lastProject != "p1" {
		t.Fatalf("args %q %q", r.lastProject, r.lastUser)
	}
}

func TestUsecase_Execute_MemberSees(t *testing.T) {
	d, err := NewUsecase(newRepoFound("member"), &fakeCounter{}).Execute(ctxFor("developer"), "p1")
	if err != nil || d.AccessLevel != base.LevelMember || d.MyRole == nil || *d.MyRole != "member" {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestUsecase_Execute_SuperAdminNotMember(t *testing.T) {
	d, err := NewUsecase(newRepoFound(""), &fakeCounter{}).Execute(ctxFor("super_admin"), "p1")
	if err != nil || d.AccessLevel != base.LevelAdmin || d.MyRole != nil || d.IsMember {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestUsecase_Execute_NonMemberIs404(t *testing.T) {
	r, c := newRepoFound(""), &fakeCounter{}
	_, err := NewUsecase(r, c).Execute(ctxFor("developer"), "p1")
	if err != base.ErrProjectNotFound {
		t.Fatalf("err %v", err)
	}
	if r.statsCalls != 0 || c.calls != 0 {
		t.Fatalf("stats=%d counts=%d", r.statsCalls, c.calls)
	}
}

func TestUsecase_Execute_NotFound(t *testing.T) {
	r := &fakeRepo{found: false}
	_, err := NewUsecase(r, &fakeCounter{}).Execute(ctxFor("developer"), "x")
	if err != base.ErrProjectNotFound {
		t.Fatalf("err %v", err)
	}
}

func TestUsecase_Execute_NoIdentity(t *testing.T) {
	r := newRepoFound("owner")
	_, err := NewUsecase(r, &fakeCounter{}).Execute(context.Background(), "p1")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized || r.findCalls != 0 {
		t.Fatalf("err %v calls %d", err, r.findCalls)
	}
}

func TestUsecase_Execute_FindAccessError(t *testing.T) {
	boom := errors.New("boom")
	r := &fakeRepo{findErr: boom}
	_, err := NewUsecase(r, &fakeCounter{}).Execute(ctxFor("developer"), "p1")
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) {
		t.Fatalf("err %v", err)
	}
}

func TestUsecase_Execute_MemberStatsError(t *testing.T) {
	boom := errors.New("boom")
	r, c := newRepoFound("owner"), &fakeCounter{}
	r.statsErr = boom
	_, err := NewUsecase(r, c).Execute(ctxFor("developer"), "p1")
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) || c.calls != 0 {
		t.Fatalf("err %v counts %d", err, c.calls)
	}
}

func TestUsecase_Execute_CountsFailSoft(t *testing.T) {
	d, err := NewUsecase(newRepoFound("owner"), &fakeCounter{out: map[string]base.ContentCounts{}}).Execute(ctxFor("developer"), "p1")
	if err != nil || d.Counts.Files != nil || d.Counts.Specs != nil || d.Counts.Keys != nil {
		t.Fatalf("%+v %v", d, err)
	}
	three := 3
	c := &fakeCounter{out: map[string]base.ContentCounts{"p1": {Files: &three}}}
	d, err = NewUsecase(newRepoFound("owner"), c).Execute(ctxFor("developer"), "p1")
	if err != nil || d.Counts.Files == nil || *d.Counts.Files != 3 || d.Counts.Specs != nil || d.Counts.Keys != nil {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestUsecase_Execute_CountsCalledOnceWithProjectID(t *testing.T) {
	c := &fakeCounter{}
	if _, err := NewUsecase(newRepoFound("owner"), c).Execute(ctxFor("developer"), "p1"); err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 || len(c.ids) != 1 || c.ids[0] != "p1" {
		t.Fatalf("%d %v", c.calls, c.ids)
	}
}

func TestUsecase_Execute_ZeroOwners(t *testing.T) {
	r := newRepoFound("")
	r.members, r.owners = 0, 0
	d, err := NewUsecase(r, &fakeCounter{}).Execute(ctxFor("super_admin"), "p1")
	if err != nil || d.OwnerCount != 0 || d.MemberCount != 0 {
		t.Fatalf("%+v %v", d, err)
	}
}
