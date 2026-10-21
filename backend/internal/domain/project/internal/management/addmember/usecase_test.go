package addmember

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
)

type env struct {
	inTx   bool
	events []string
}

type fakeRepo struct {
	e          *env
	p          base.Project
	memberRole string
	found      bool
	lockOK     bool
	members    []base.Member
	inserted   bool
	inserts    []base.Member

	errFind, errLock, errMembers, errInsert error
	inTxCalls, insertCalls                  int
}

func (f *fakeRepo) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	f.e.events = append(f.e.events, "find")
	return f.p, f.memberRole, f.found, f.errFind
}

func (f *fakeRepo) InTx(_ context.Context, fn func(Tx) error) error {
	f.inTxCalls++
	f.e.events = append(f.e.events, "intx")
	f.e.inTx = true
	defer func() { f.e.inTx = false }()
	return fn(f)
}

func (f *fakeRepo) LockProject(context.Context, string) (base.Project, bool, error) {
	return f.p, f.lockOK, f.errLock
}
func (f *fakeRepo) LockMembers(context.Context, string) ([]base.Member, error) {
	return f.members, f.errMembers
}
func (f *fakeRepo) InsertMember(_ context.Context, m base.Member) (bool, error) {
	f.insertCalls++
	f.inserts = append(f.inserts, m)
	return f.inserted, f.errInsert
}

type fakeUsers struct {
	e     *env
	resp  getuser.Response
	err   error
	calls int
	inTx  bool
}

func (u *fakeUsers) Get(context.Context, string) (getuser.Response, error) {
	u.calls++
	u.inTx = u.inTx || u.e.inTx
	u.e.events = append(u.e.events, "get")
	return u.resp, u.err
}
func (u *fakeUsers) FindDevelopers(context.Context, string, int) ([]finddevelopers.Developer, error) {
	return nil, nil
}

type fakeAudit struct {
	e    *env
	evs  []audit.Event
	inTx bool
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	a.inTx = a.inTx || a.e.inTx
	a.e.events = append(a.e.events, "audit")
	a.evs = append(a.evs, ev)
}

const (
	pid    = "p1"
	target = "0192f5a0-cccc-7aaa-8bbb-00000000000c"
)

var now = time.Date(2026, 10, 22, 9, 10, 0, 0, time.UTC)

func ctxFor(uid, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: uid, Role: role, Email: uid + "@x"})
}

func setup(actorRole string) (*Usecase, *fakeRepo, *fakeUsers, *fakeAudit) {
	e := &env{}
	r := &fakeRepo{
		e: e, p: base.Project{ID: pid, Name: "Payments"}, memberRole: actorRole, found: true, lockOK: true, inserted: true,
		members: []base.Member{{UserID: "a", Role: actorRole}},
	}
	u := &fakeUsers{e: e, resp: getuser.Response{Found: true, ID: target, Name: "Citra", Email: "c@tim.id", Role: "developer", IsActive: true}}
	a := &fakeAudit{e: e}
	return NewUsecase(r, u, a, func() time.Time { return now }), r, u, a
}

func reqFor(role string) AddMemberRequest { return AddMemberRequest{UserID: target, Role: role} }

func assertNoSideEffects(t *testing.T, r *fakeRepo, u *fakeUsers, a *fakeAudit, wantGet int) {
	t.Helper()
	if u.calls != wantGet || r.insertCalls != 0 || len(a.evs) != 0 {
		t.Fatalf("get=%d insert=%d audit=%d", u.calls, r.insertCalls, len(a.evs))
	}
}

func TestExecute_Success_Owner(t *testing.T) {
	uc, r, u, a := setup("owner")
	meta := base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"}
	dto, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), meta)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"find", "get", "intx", "audit"}
	if len(r.e.events) != len(want) {
		t.Fatalf("events %v", r.e.events)
	}
	for i := range want {
		if r.e.events[i] != want[i] {
			t.Fatalf("events %v", r.e.events)
		}
	}
	if u.inTx || a.inTx {
		t.Fatal("bus/audit dipanggil di dalam tx")
	}
	m := r.inserts[0]
	if m.Role != base.RoleMember || m.UserID != target || *m.AddedBy != "a" || *m.AddedByLabel != "a@x" || !m.AddedAt.Equal(now) {
		t.Fatalf("%+v", m)
	}
	if !dto.Found || !dto.IsActive || *dto.Name != "Citra" || *dto.Email != "c@tim.id" || dto.Role != "member" {
		t.Fatalf("%+v", dto)
	}
	ev := a.evs[0]
	if len(a.evs) != 1 || ev.Action != record.ActionProjectMemberAdded || ev.ProjectID != pid || ev.ProjectLabel != "Payments" ||
		ev.TargetType != "project_member" || ev.Target != target || ev.Meta != meta {
		t.Fatalf("%+v", ev)
	}
	if len(ev.Args) != 3 || ev.Args["user_id"] != target || ev.Args["user_label"] != "c@tim.id" || ev.Args["role"] != "member" {
		t.Fatalf("%+v", ev.Args)
	}
}

func TestExecute_OwnerRole(t *testing.T) {
	uc, r, _, _ := setup("owner")
	if _, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor("owner"), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if r.inserts[0].Role != base.RoleOwner {
		t.Fatalf("%+v", r.inserts[0])
	}
}

func TestT03_AC7_AdminAssignsOwnerOnOwnerlessProject(t *testing.T) {
	uc, r, _, a := setup("")
	r.memberRole, r.found, r.members = "", true, nil
	dto, err := uc.Execute(ctxFor("admin", "super_admin"), pid, reqFor("owner"), base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if dto.Role != "owner" || *r.inserts[0].AddedBy != "admin" || len(a.evs) != 1 {
		t.Fatalf("%+v", dto)
	}
}

func TestT03_AC2_MemberCannotAdd(t *testing.T) {
	uc, r, u, a := setup("member")
	_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	if !errors.Is(err, base.ErrForbidden) {
		t.Fatal(err)
	}
	assertNoSideEffects(t, r, u, a, 0)
	if r.inTxCalls != 0 {
		t.Fatal("intx dipanggil")
	}
}

func TestExecute_NonMemberAndMissingProject(t *testing.T) {
	uc, r, u, a := setup("")
	r.found = false
	_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatal(err)
	}
	assertNoSideEffects(t, r, u, a, 0)

	uc, r, u, a = setup("")
	_, err = uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatal(err)
	}
	assertNoSideEffects(t, r, u, a, 0)
}

func TestT03_AC4_IneligibleCandidates(t *testing.T) {
	cases := map[string]getuser.Response{
		base.ReasonUserNotFound:     {Found: false},
		base.ReasonUserNotDeveloper: {Found: true, ID: target, Role: "super_admin", IsActive: true},
		"sa_inactive":               {Found: true, ID: target, Role: "super_admin", IsActive: false},
		base.ReasonUserInactive:     {Found: true, ID: target, Role: "developer", IsActive: false},
	}
	for name, resp := range cases {
		want := name
		if name == "sa_inactive" {
			want = base.ReasonUserNotDeveloper
		}
		uc, r, u, a := setup("owner")
		u.resp = resp
		_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
			t.Fatalf("%s: %v", name, err)
		}
		if d, _ := e.Details.(map[string]any); d["reason"] != want {
			t.Fatalf("%s: %+v", name, e.Details)
		}
		assertNoSideEffects(t, r, u, a, 1)
		if r.inTxCalls != 0 {
			t.Fatalf("%s: intx", name)
		}
	}
}

func TestExecute_InvalidRoleBeforeBus(t *testing.T) {
	for _, role := range []string{"admin", "OWNER"} {
		uc, r, u, a := setup("owner")
		_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(role), base.RequestMeta{})
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
			t.Fatalf("%s: %v", role, err)
		}
		assertNoSideEffects(t, r, u, a, 0)
	}
}

func TestExecute_AlreadyMemberInLockedList(t *testing.T) {
	uc, r, u, a := setup("owner")
	r.members = append(r.members, base.Member{UserID: target, Role: "member"})
	_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	if !errors.Is(err, base.ErrAlreadyMember) {
		t.Fatal(err)
	}
	assertNoSideEffects(t, r, u, a, 1)
}

func TestExecute_InsertNotInserted(t *testing.T) {
	uc, r, _, a := setup("owner")
	r.inserted = false
	_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	if !errors.Is(err, base.ErrAlreadyMember) || len(a.evs) != 0 {
		t.Fatal(err)
	}
}

func TestExecute_ActorDemotedInsideTx(t *testing.T) {
	uc, r, _, a := setup("owner")
	r.members = []base.Member{{UserID: "a", Role: "member"}}
	_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	if !errors.Is(err, base.ErrForbidden) || r.insertCalls != 0 || len(a.evs) != 0 {
		t.Fatal(err)
	}
}

func TestExecute_ActorRemovedInsideTx(t *testing.T) {
	uc, r, _, a := setup("owner")
	r.members = nil
	_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	if !errors.Is(err, base.ErrProjectNotFound) || r.insertCalls != 0 || len(a.evs) != 0 {
		t.Fatal(err)
	}
}

func TestExecute_LockProjectMissing(t *testing.T) {
	uc, r, _, a := setup("owner")
	r.lockOK = false
	_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	if !errors.Is(err, base.ErrProjectNotFound) || len(a.evs) != 0 {
		t.Fatal(err)
	}
}

func TestExecute_AdminPassesRecheckWithoutMembership(t *testing.T) {
	uc, r, _, _ := setup("")
	r.found, r.members = true, nil
	if _, err := uc.Execute(ctxFor("admin", "super_admin"), pid, reqFor(""), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_UsersErrorWrapped(t *testing.T) {
	uc, r, u, a := setup("owner")
	u.err = bus.ErrNotRegistered
	_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
	var e *errs.Error
	if err == nil || errors.As(err, &e) || !errors.Is(err, bus.ErrNotRegistered) || r.inTxCalls != 0 || len(a.evs) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestExecute_InfraErrorsWrapped(t *testing.T) {
	boom := errors.New("boom")
	mods := map[string]func(*fakeRepo){
		"find":    func(r *fakeRepo) { r.errFind = boom },
		"lock":    func(r *fakeRepo) { r.errLock = boom },
		"members": func(r *fakeRepo) { r.errMembers = boom },
		"insert":  func(r *fakeRepo) { r.errInsert = boom },
	}
	for name, mod := range mods {
		uc, r, _, a := setup("owner")
		mod(r)
		_, err := uc.Execute(ctxFor("a", "developer"), pid, reqFor(""), base.RequestMeta{})
		var e *errs.Error
		if !errors.Is(err, boom) || errors.As(err, &e) || len(a.evs) != 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestExecute_CanonicalTargetID(t *testing.T) {
	uc, r, _, a := setup("owner")
	_, err := uc.Execute(ctxFor("a", "developer"), pid, AddMemberRequest{UserID: "0192F5A0-CCCC-7AAA-8BBB-00000000000C"}, base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if r.inserts[0].UserID != target || a.evs[0].Target != target {
		t.Fatalf("%+v %+v", r.inserts[0], a.evs[0])
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	uc, _, _, _ := setup("owner")
	_, err := uc.Execute(context.Background(), pid, reqFor(""), base.RequestMeta{})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatal(err)
	}
}
