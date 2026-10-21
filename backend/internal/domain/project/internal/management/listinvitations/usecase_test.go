package listinvitations

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/inviteuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
)

type fakeRepo struct {
	role                 string
	found                bool
	findErr, listErr     error
	members              []base.Member
	findCalls, listCalls int
}

func (f *fakeRepo) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	f.findCalls++
	return base.Project{ID: "p1"}, f.role, f.found, f.findErr
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

func (f *fakeUsers) GetByEmail(context.Context, string) (getuser.Response, error) {
	return getuser.Response{}, nil
}

func (f *fakeUsers) Invite(context.Context, inviteuser.Request) (inviteuser.Response, error) {
	return inviteuser.Response{}, nil
}

func (f *fakeUsers) ResendInvitation(context.Context, resendinvitation.Request) (resendinvitation.Response, error) {
	return resendinvitation.Response{}, nil
}

func ctxFor(role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "actor", Role: role, Email: "a@x.id"})
}

func mem(uid string, addedAt time.Time) base.Member {
	label := "Owner"
	return base.Member{ProjectID: "p1", UserID: uid, Role: "member", AddedByLabel: &label, AddedAt: addedAt}
}

func pending(id string) getuser.Response {
	return getuser.Response{Found: true, ID: id, Name: id, Email: id + "@x.id", Role: "developer", IsActive: true,
		Invitation: &getuser.Invitation{ExpiresAt: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), DeliveryStatus: "sent"}}
}

func plain(id string) getuser.Response {
	return getuser.Response{Found: true, ID: id, Name: id, Email: id + "@x.id", Role: "developer", IsActive: true}
}

func found(role string) *fakeRepo { return &fakeRepo{role: role, found: true} }

var t0 = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

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

func TestExecute_NonMemberGets404(t *testing.T) {
	r, u := found(""), &fakeUsers{}
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, base.ErrProjectNotFound) || r.listCalls != 0 || len(u.calls) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestExecute_MemberForbidden(t *testing.T) {
	r, u := found("member"), &fakeUsers{}
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, base.ErrForbidden) || r.listCalls != 0 || len(u.calls) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestExecute_OwnerAndSuperAdminAllowed(t *testing.T) {
	for _, c := range []struct{ role, member string }{{"developer", "owner"}, {"super_admin", ""}} {
		r, u := found(c.member), &fakeUsers{}
		if _, err := NewUsecase(r, r, u).Execute(ctxFor(c.role), "p1"); err != nil {
			t.Fatalf("%s: %v", c.role, err)
		}
	}
}

func TestExecute_FiltersPendingOnly(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", t0), mem("u2", t0), mem("u3", t0), mem("u4", t0), mem("u5", t0)}
	u := &fakeUsers{data: map[string]getuser.Response{
		"u1": pending("u1"), "u2": plain("u2"), "u3": pending("u3"),
		"u4": {Found: false}, "u5": plain("u5")}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if err != nil || len(res.Items) != 2 {
		t.Fatalf("%v %v", res, err)
	}
	if res.Items[0].UserID != "u1" || res.Items[1].UserID != "u3" {
		t.Fatalf("%v", res.Items)
	}
}

func TestExecute_SortedByAddedAtDesc(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", t0), mem("u2", t0.Add(time.Hour)), mem("u3", t0.Add(2*time.Hour))}
	u := &fakeUsers{data: map[string]getuser.Response{"u1": pending("u1"), "u2": pending("u2"), "u3": pending("u3")}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if err != nil || len(res.Items) != 3 || res.Items[0].UserID != "u3" || res.Items[2].UserID != "u1" {
		t.Fatalf("%v %v", res, err)
	}
}

func TestExecute_RepoErrorsWrapped(t *testing.T) {
	boom := errors.New("boom")
	r, u := found("owner"), &fakeUsers{}
	r.findErr = boom
	_, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, boom) || errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("%v", err)
	}
	r2 := found("owner")
	r2.listErr = boom
	if _, err := NewUsecase(r2, r2, u).Execute(ctxFor("developer"), "p1"); !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
}

func TestExecute_GetUserErrorFailsAll(t *testing.T) {
	boom := errors.New("boom")
	r := found("owner")
	r.members = []base.Member{mem("u1", t0), mem("u2", t0), mem("u3", t0)}
	u := &fakeUsers{data: map[string]getuser.Response{"u1": pending("u1")}, errFor: map[string]error{"u2": boom}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if !errors.Is(err, boom) || len(res.Items) != 0 || len(u.calls) != 2 {
		t.Fatalf("%v %v %v", err, res, u.calls)
	}
}

func TestExecute_ContextCancelledMidLoop(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", t0), mem("u2", t0), mem("u3", t0)}
	ctx, cancel := context.WithCancel(ctxFor("developer"))
	u := &fakeUsers{data: map[string]getuser.Response{"u1": pending("u1")}, onCall: func(n int) {
		if n == 1 {
			cancel()
		}
	}}
	_, err := NewUsecase(r, r, u).Execute(ctx, "p1")
	if !errors.Is(err, context.Canceled) || len(u.calls) != 1 {
		t.Fatalf("%v %v", err, u.calls)
	}
}

func TestExecute_NoInvitationsEmptyNotError(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", t0)}
	u := &fakeUsers{data: map[string]getuser.Response{"u1": plain("u1")}}
	res, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1")
	if err != nil || res.Items == nil || len(res.Items) != 0 {
		t.Fatalf("%#v %v", res, err)
	}
}

func TestExecute_GetCalledOncePerMemberInOrder(t *testing.T) {
	r := found("owner")
	r.members = []base.Member{mem("u1", t0), mem("u2", t0), mem("u3", t0)}
	u := &fakeUsers{data: map[string]getuser.Response{}}
	if _, err := NewUsecase(r, r, u).Execute(ctxFor("developer"), "p1"); err != nil {
		t.Fatal(err)
	}
	if len(u.calls) != 3 || u.calls[0] != "u1" || u.calls[1] != "u2" || u.calls[2] != "u3" {
		t.Fatalf("%v", u.calls)
	}
}
