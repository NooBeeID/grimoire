package listmembers

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
)

type fakeRepo struct {
	role                    string
	found                   bool
	findErr, listErr        error
	members                 []base.Member
	findCalls, listCalls    int
	lastProjectID, lastUser string
}

func (f *fakeRepo) FindAccess(_ context.Context, pid, uid string) (base.Project, string, bool, error) {
	f.findCalls++
	f.lastProjectID, f.lastUser = pid, uid
	return base.Project{ID: pid}, f.role, f.found, f.findErr
}

func (f *fakeRepo) ListMembers(context.Context, string) ([]base.Member, error) {
	f.listCalls++
	return f.members, f.listErr
}

type fakeUsers struct {
	data   map[string]getuser.Response
	errFor map[string]error
	calls  []string
	onCall func(n int)
}

func (f *fakeUsers) Get(_ context.Context, id string) (getuser.Response, error) {
	f.calls = append(f.calls, id)
	if f.onCall != nil {
		f.onCall(len(f.calls))
	}
	if err := f.errFor[id]; err != nil {
		return getuser.Response{}, err
	}
	return f.data[id], nil
}

func (f *fakeUsers) FindDevelopers(context.Context, string, int) ([]finddevelopers.Developer, error) {
	return nil, nil
}

func ctxFor(role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "actor", Role: role, Email: "a@x.id"})
}

func mem(uid, role string) base.Member {
	by, label := "actor", "a@x.id"
	return base.Member{ProjectID: "p1", UserID: uid, Role: role, AddedBy: &by, AddedByLabel: &label,
		AddedAt: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}
}

func usr(id, name string, active bool) getuser.Response {
	return getuser.Response{Found: true, ID: id, Name: name, Email: name + "@x.id", Role: "developer", IsActive: active}
}

func found(role string) *fakeRepo { return &fakeRepo{role: role, found: true} }

func TestExecute_NoIdentity(t *testing.T) {
	r, u := found("owner"), &fakeUsers{}
	_, err := NewUsecase(r, r, u).Execute(context.Background(), "p1")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized || r.findCalls+r.listCalls+len(u.calls) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestExecute_ProjectNotFound(t *testing.T) {
	r, u := &fakeRepo{}, &fakeUsers{}
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, base.ErrProjectNotFound) || r.listCalls != 0 || len(u.calls) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestExecute_NonMemberDeveloperGets404(t *testing.T) {
	r, u := found(""), &fakeUsers{}
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, base.ErrProjectNotFound) || r.listCalls != 0 || len(u.calls) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestT03_AC2_MemberCanListMembers(t *testing.T) {
	r, u := found("member"), &fakeUsers{}
	if _, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1"); err != nil || r.listCalls != 1 {
		t.Fatalf("%v %d", err, r.listCalls)
	}
}

func TestExecute_OwnerCanList(t *testing.T) {
	r, u := found("owner"), &fakeUsers{}
	if _, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1"); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_SuperAdminNotMember(t *testing.T) {
	r := found("")
	r.members = []base.Member{mem("u1", "owner")}
	u := &fakeUsers{data: map[string]getuser.Response{"u1": usr("u1", "Ani", true)}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("super_admin"), "p1")
	if err != nil || len(res.Items) != 1 || len(u.calls) != 1 || u.calls[0] != "u1" {
		t.Fatalf("%v %v %v", err, res, u.calls)
	}
}

func TestExecute_EnrichesEveryMember(t *testing.T) {
	r := found("member")
	r.members = []base.Member{mem("u1", "owner"), mem("u2", "member"), mem("u3", "member")}
	u := &fakeUsers{data: map[string]getuser.Response{
		"u1": usr("u1", "A", true), "u2": usr("u2", "B", true), "u3": usr("u3", "C", true)}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if err != nil || len(u.calls) != 3 || len(res.Items) != 3 {
		t.Fatalf("%v %v", err, u.calls)
	}
	d := res.Items[0]
	if d.UserID != "u1" || d.Name == nil || *d.Name != "A" || d.Email == nil || !d.IsActive || !d.Found ||
		d.Role != "owner" || d.AddedAt.IsZero() || d.AddedBy == nil || d.AddedByLabel == nil {
		t.Fatalf("%+v", d)
	}
}

func TestExecute_SortsOwnerFirstThenName(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", "member"), mem("u2", "owner"), mem("u3", "member"), mem("u4", "owner")}
	u := &fakeUsers{data: map[string]getuser.Response{
		"u1": usr("u1", "budi", true), "u2": usr("u2", "Zed", true),
		"u3": usr("u3", "Andi", true), "u4": usr("u4", "adi", true)}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"u4", "u2", "u3", "u1"}
	for i, id := range want {
		if res.Items[i].UserID != id {
			t.Fatalf("pos %d: %s want %s", i, res.Items[i].UserID, id)
		}
	}
}

func TestExecute_UnknownUserKeptWithFoundFalse(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", "member"), mem("u2", "member")}
	u := &fakeUsers{data: map[string]getuser.Response{"u2": usr("u2", "Ani", true)}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if err != nil || len(res.Items) != 2 {
		t.Fatalf("%v", err)
	}
	last := res.Items[1]
	if last.UserID != "u1" || last.Found || last.Name != nil || last.Email != nil || last.IsActive {
		t.Fatalf("%+v", last)
	}
}

func TestExecute_InactiveUserListed(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", "member")}
	u := &fakeUsers{data: map[string]getuser.Response{"u1": usr("u1", "Ani", false)}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if err != nil || len(res.Items) != 1 || res.Items[0].IsActive || !res.Items[0].Found {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestExecute_EmptyMembersReturnsEmptySlice(t *testing.T) {
	r, u := found("owner"), &fakeUsers{}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if err != nil || res.Items == nil || len(res.Items) != 0 || len(u.calls) != 0 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestExecute_GetUserErrorFailsWhole(t *testing.T) {
	boom := errors.New("boom")
	r := found("owner")
	r.members = []base.Member{mem("u1", "member"), mem("u2", "member"), mem("u3", "member")}
	u := &fakeUsers{data: map[string]getuser.Response{"u1": usr("u1", "A", true)}, errFor: map[string]error{"u2": boom}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, boom) || res.Items != nil || len(u.calls) != 2 {
		t.Fatalf("%v %v %v", err, res, u.calls)
	}
}

func TestExecute_GetUserNotRegistered(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", "member")}
	u := &fakeUsers{errFor: map[string]error{"u1": bus.ErrNotRegistered}}
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	var e *errs.Error
	if !errors.Is(err, bus.ErrNotRegistered) || errors.As(err, &e) {
		t.Fatalf("%v", err)
	}
}

func TestExecute_FindAccessErrorWrapped(t *testing.T) {
	boom := errors.New("db")
	r := &fakeRepo{findErr: boom}
	u := &fakeUsers{}
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, boom) || len(u.calls) != 0 || r.listCalls != 0 {
		t.Fatalf("%v", err)
	}
}

func TestExecute_ListMembersErrorWrapped(t *testing.T) {
	boom := errors.New("db")
	r := found("owner")
	r.listErr = boom
	u := &fakeUsers{}
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, boom) || len(u.calls) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestExecute_ContextCancelledStopsEnrichment(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", "member"), mem("u2", "member"), mem("u3", "member")}
	cctx, cancel := context.WithCancel(ctxFor("developer"))
	u := &fakeUsers{data: map[string]getuser.Response{"u1": usr("u1", "A", true)}, onCall: func(n int) {
		if n == 1 {
			cancel()
		}
	}}
	_, err := NewUsecase(r, r, u).Execute(cctx, "p1")
	if !errors.Is(err, context.Canceled) || len(u.calls) != 1 {
		t.Fatalf("%v %v", err, u.calls)
	}
}

func TestExecute_ProjectIDNotUUIDPassedToStore(t *testing.T) {
	r, u := &fakeRepo{}, &fakeUsers{}
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "bukan-uuid")
	if !errors.Is(err, base.ErrProjectNotFound) || r.lastProjectID != "bukan-uuid" || r.lastUser != "actor" {
		t.Fatalf("%v %q %q", err, r.lastProjectID, r.lastUser)
	}
}
