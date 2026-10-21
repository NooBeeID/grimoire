package deleteproject

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

type fakeRepo struct {
	p          base.Project
	memberRole string
	found      bool
	locked     base.Project
	lockFound  bool
	members    []base.Member
	deleted    bool

	errFind, errLock, errMembers, errDelete, errTx error

	deleteCalls int
	inTxCalls   int
	inTx        bool
	events      *[]string
}

func (f *fakeRepo) note(s string) {
	if f.events != nil {
		*f.events = append(*f.events, s)
	}
}

func (f *fakeRepo) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	return f.p, f.memberRole, f.found, f.errFind
}

func (f *fakeRepo) InTx(_ context.Context, fn func(txRepo) error) error {
	f.inTxCalls++
	f.note("intx")
	if f.errTx != nil {
		return f.errTx
	}
	f.inTx = true
	defer func() { f.inTx = false }()
	return fn(f)
}

func (f *fakeRepo) LockProject(context.Context, string) (base.Project, bool, error) {
	return f.locked, f.lockFound, f.errLock
}

func (f *fakeRepo) LockMembers(context.Context, string) ([]base.Member, error) {
	return f.members, f.errMembers
}

func (f *fakeRepo) DeleteProject(context.Context, string) (bool, error) {
	f.deleteCalls++
	return f.deleted, f.errDelete
}

type fakeCounts struct {
	cc     base.ContentCounts
	calls  int
	repo   *fakeRepo
	inTxAt bool
	events *[]string
}

func (c *fakeCounts) ForProjects(_ context.Context, ids []string) map[string]base.ContentCounts {
	c.calls++
	if c.repo != nil {
		c.inTxAt = c.repo.inTx
	}
	if c.events != nil {
		*c.events = append(*c.events, "counts")
	}
	return map[string]base.ContentCounts{ids[0]: c.cc}
}

type fakeAudit struct {
	evs    []audit.Event
	repo   *fakeRepo
	inTxAt bool
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	if a.repo != nil {
		a.inTxAt = a.repo.inTx
	}
	a.evs = append(a.evs, ev)
}

func ip(i int) *int { return &i }

const pid = "p1"

func newRepo2(name, memberRole string) *fakeRepo {
	p := base.Project{ID: pid, Name: name}
	return &fakeRepo{
		p: p, memberRole: memberRole, found: true, locked: p, lockFound: true, deleted: true,
		members: []base.Member{{UserID: "u1", Role: base.RoleOwner}, {UserID: "u2", Role: base.RoleMember}},
	}
}

func ctxFor(uid, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: uid, Role: role, Email: uid + "@x"})
}

func newUC(r *fakeRepo) (*Usecase, *fakeCounts, *fakeAudit) {
	c := &fakeCounts{repo: r}
	a := &fakeAudit{repo: r}
	return NewUsecase(r, c, a), c, a
}

func in(name string) Input {
	return Input{ProjectID: pid, ConfirmName: name, Meta: base.RequestMeta{IP: "1.1.1.1"}}
}

func TestExecute_Success_Owner(t *testing.T) {
	r := newRepo2("Payments", "owner")
	uc, c, a := newUC(r)
	c.cc = base.ContentCounts{Files: ip(3), Specs: ip(1)}
	if err := uc.Execute(ctxFor("u1", "developer"), in("Payments")); err != nil {
		t.Fatal(err)
	}
	if r.deleteCalls != 1 || len(a.evs) != 1 {
		t.Fatalf("%d %d", r.deleteCalls, len(a.evs))
	}
	ev := a.evs[0]
	if ev.Action != record.ActionProjectDeleted || ev.ProjectID != pid || ev.ProjectLabel != "Payments" ||
		ev.TargetType != "project" || ev.Target != pid || ev.Meta.IP != "1.1.1.1" {
		t.Fatalf("%+v", ev)
	}
	if ev.Args["name"] != "Payments" || ev.Args["member_count"] != 2 {
		t.Fatalf("%+v", ev.Args)
	}
	cm := ev.Args["counts"].(map[string]any)
	if cm["files"] != 3 || cm["specs"] != 1 || cm["keys"] != nil {
		t.Fatalf("%+v", cm)
	}
	if len(ev.Args) != 3 {
		t.Fatalf("args tak terduga: %+v", ev.Args)
	}
	if c.inTxAt || a.inTxAt {
		t.Fatal("bus dipanggil di dalam tx")
	}
}

func TestExecute_Success_SuperAdminNotMember(t *testing.T) {
	r := newRepo2("Payments", "")
	uc, _, a := newUC(r)
	if err := uc.Execute(ctxFor("sa", "super_admin"), in("Payments")); err != nil {
		t.Fatal(err)
	}
	if len(a.evs) != 1 || a.evs[0].Args["member_count"] != 2 {
		t.Fatalf("%+v", a.evs)
	}
}

func assertNoEffects(t *testing.T, r *fakeRepo, c *fakeCounts, a *fakeAudit) {
	t.Helper()
	if r.deleteCalls != 0 || len(a.evs) != 0 {
		t.Fatalf("delete=%d audit=%d", r.deleteCalls, len(a.evs))
	}
	_ = c
}

func TestExecute_MemberForbidden(t *testing.T) {
	r := newRepo2("Payments", "member")
	uc, c, a := newUC(r)
	if err := uc.Execute(ctxFor("u2", "developer"), in("Payments")); err != base.ErrForbidden {
		t.Fatal(err)
	}
	assertNoEffects(t, r, c, a)
	if c.calls != 0 || r.inTxCalls != 0 {
		t.Fatal("efek samping")
	}
}

func TestExecute_NonMemberNotFound(t *testing.T) {
	r := newRepo2("Payments", "")
	uc, c, a := newUC(r)
	if err := uc.Execute(ctxFor("dev", "developer"), in("Payments")); err != base.ErrProjectNotFound {
		t.Fatal(err)
	}
	assertNoEffects(t, r, c, a)
	if c.calls != 0 || r.inTxCalls != 0 {
		t.Fatal("efek samping")
	}
}

func TestExecute_UnknownProject(t *testing.T) {
	r := newRepo2("Payments", "owner")
	r.found = false
	uc, _, _ := newUC(r)
	if err := uc.Execute(ctxFor("u1", "developer"), in("Payments")); err != base.ErrProjectNotFound {
		t.Fatal(err)
	}
}

func TestExecute_ConfirmationMismatch(t *testing.T) {
	for _, name := range []string{"payments", "Payments ", ""} {
		r := newRepo2("Payments", "owner")
		uc, c, a := newUC(r)
		if err := uc.Execute(ctxFor("u1", "developer"), in(name)); err != base.ErrConfirmationMismatch {
			t.Fatalf("%q: %v", name, err)
		}
		assertNoEffects(t, r, c, a)
		if c.calls != 0 || r.inTxCalls != 0 {
			t.Fatal("efek samping")
		}
	}
}

func TestExecute_MemberWithWrongNameGets403NotMismatch(t *testing.T) {
	r := newRepo2("Payments", "member")
	uc, _, _ := newUC(r)
	if err := uc.Execute(ctxFor("u2", "developer"), in("salah")); err != base.ErrForbidden {
		t.Fatal(err)
	}
}

func TestExecute_NameChangedBeforeLock(t *testing.T) {
	r := newRepo2("Payments", "owner")
	r.locked.Name = "Baru"
	uc, _, a := newUC(r)
	if err := uc.Execute(ctxFor("u1", "developer"), in("Payments")); err != base.ErrConfirmationMismatch {
		t.Fatal(err)
	}
	if r.deleteCalls != 0 || len(a.evs) != 0 {
		t.Fatal("efek samping")
	}
}

func TestExecute_ActorDemotedBeforeLock(t *testing.T) {
	r := newRepo2("Payments", "owner")
	r.members = []base.Member{{UserID: "u1", Role: base.RoleMember}, {UserID: "u2", Role: base.RoleOwner}}
	uc, _, a := newUC(r)
	if err := uc.Execute(ctxFor("u1", "developer"), in("Payments")); err != base.ErrForbidden {
		t.Fatal(err)
	}
	if r.deleteCalls != 0 || len(a.evs) != 0 {
		t.Fatal("efek samping")
	}
}

func TestExecute_ActorRemovedBeforeLock(t *testing.T) {
	r := newRepo2("Payments", "owner")
	r.members = []base.Member{{UserID: "u2", Role: base.RoleOwner}}
	uc, _, _ := newUC(r)
	if err := uc.Execute(ctxFor("u1", "developer"), in("Payments")); err != base.ErrProjectNotFound {
		t.Fatal(err)
	}
}

func TestExecute_ProjectGoneBeforeLock(t *testing.T) {
	r := newRepo2("Payments", "owner")
	r.lockFound = false
	uc, _, a := newUC(r)
	if err := uc.Execute(ctxFor("u1", "developer"), in("Payments")); err != base.ErrProjectNotFound {
		t.Fatal(err)
	}
	r2 := newRepo2("Payments", "owner")
	r2.deleted = false
	uc, _, a2 := newUC(r2)
	if err := uc.Execute(ctxFor("u1", "developer"), in("Payments")); err != base.ErrProjectNotFound {
		t.Fatal(err)
	}
	if len(a.evs)+len(a2.evs) != 0 {
		t.Fatal("audit")
	}
}

func TestExecute_CountsFailSoft(t *testing.T) {
	var events []string
	r := newRepo2("Payments", "owner")
	r.events = &events
	uc, c, a := newUC(r)
	c.events = &events
	if err := uc.Execute(ctxFor("u1", "developer"), in("Payments")); err != nil {
		t.Fatal(err)
	}
	cm := a.evs[0].Args["counts"].(map[string]any)
	if cm["files"] != nil || cm["specs"] != nil || cm["keys"] != nil {
		t.Fatalf("%+v", cm)
	}
	if len(events) != 2 || events[0] != "counts" || events[1] != "intx" {
		t.Fatalf("urutan: %v", events)
	}
}

func TestExecute_CountsPassedThrough(t *testing.T) {
	r := newRepo2("Payments", "owner")
	uc, c, a := newUC(r)
	c.cc = base.ContentCounts{Files: ip(3), Specs: ip(1)}
	_ = uc.Execute(ctxFor("u1", "developer"), in("Payments"))
	cm := a.evs[0].Args["counts"].(map[string]any)
	if cm["files"] != 3 || cm["specs"] != 1 || cm["keys"] != nil {
		t.Fatalf("%+v", cm)
	}
}

func TestExecute_RepoErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]func(*fakeRepo){
		"find":    func(r *fakeRepo) { r.errFind = boom },
		"lock":    func(r *fakeRepo) { r.errLock = boom },
		"members": func(r *fakeRepo) { r.errMembers = boom },
		"delete":  func(r *fakeRepo) { r.errDelete = boom },
		"tx":      func(r *fakeRepo) { r.errTx = boom },
	}
	for name, mut := range cases {
		r := newRepo2("Payments", "owner")
		mut(r)
		uc, _, a := newUC(r)
		err := uc.Execute(ctxFor("u1", "developer"), in("Payments"))
		var e *errs.Error
		if !errors.Is(err, boom) || errors.As(err, &e) {
			t.Fatalf("%s: %v", name, err)
		}
		if len(a.evs) != 0 {
			t.Fatalf("%s: audit", name)
		}
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	r := newRepo2("Payments", "owner")
	uc, _, _ := newUC(r)
	err := uc.Execute(context.Background(), in("Payments"))
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatal(err)
	}
}

func TestExecute_AuditArgsKeys(t *testing.T) {
	r := newRepo2("Payments", "owner")
	uc, _, a := newUC(r)
	_ = uc.Execute(ctxFor("u1", "developer"), in("Payments"))
	for k := range a.evs[0].Args {
		if k != "name" && k != "member_count" && k != "counts" {
			t.Fatalf("kunci tak diizinkan: %s", k)
		}
	}
}
