package revokeinvitation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/getuser"
)

const pid = "p1"

var (
	idA = uuid.NewString()
	idB = uuid.NewString()
	idC = uuid.NewString()
	idS = uuid.NewString()
)

type fakeRepo struct {
	memberRole string
	found      bool
	lockFound  bool
	members    []base.Member

	errFind, errLock, errMembers, errDelete error

	inTxCalls, deleteCalls int
	deleted                []string
	inTx                   bool
	events                 *[]string
}

func (f *fakeRepo) note(s string) {
	if f.events != nil {
		*f.events = append(*f.events, s)
	}
}

func (f *fakeRepo) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	f.note("find")
	return base.Project{ID: pid, Name: "Payments"}, f.memberRole, f.found, f.errFind
}

func (f *fakeRepo) InTx(_ context.Context, fn func(TxRepository) error) error {
	f.inTxCalls++
	f.note("begin")
	f.inTx = true
	err := fn(f)
	f.inTx = false
	if err == nil {
		f.note("commit")
	}
	return err
}

func (f *fakeRepo) LockProject(context.Context, string) (base.Project, bool, error) {
	f.note("lockproject")
	return base.Project{ID: pid, Name: "Payments"}, f.lockFound, f.errLock
}

func (f *fakeRepo) LockMembers(context.Context, string) ([]base.Member, error) {
	f.note("lockmembers")
	return f.members, f.errMembers
}

func (f *fakeRepo) DeleteMember(_ context.Context, _, userID string) error {
	f.note("delete")
	f.deleteCalls++
	f.deleted = append(f.deleted, userID)
	return f.errDelete
}

type fakeUsers struct {
	res    getuser.Response
	err    error
	calls  int
	gotID  string
	repo   *fakeRepo
	inTxAt bool
	events *[]string
}

func (u *fakeUsers) Get(_ context.Context, id string) (getuser.Response, error) {
	u.calls++
	u.gotID = id
	if u.repo != nil && u.repo.inTx {
		u.inTxAt = true
	}
	if u.events != nil {
		*u.events = append(*u.events, "getuser")
	}
	r := u.res
	r.ID = id
	return r, u.err
}

type fakeAudit struct {
	evs    []audit.Event
	repo   *fakeRepo
	inTxAt bool
	events *[]string
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	if a.repo != nil && a.repo.inTx {
		a.inTxAt = true
	}
	if a.events != nil {
		*a.events = append(*a.events, "audit")
	}
	a.evs = append(a.evs, ev)
}

func mem(id, role string) base.Member { return base.Member{ProjectID: pid, UserID: id, Role: role} }

type env struct {
	repo *fakeRepo
	us   *fakeUsers
	au   *fakeAudit
	uc   *Usecase
}

func newEnv(actorID string, members ...base.Member) *env {
	role := ""
	for _, m := range members {
		if m.UserID == actorID {
			role = m.Role
		}
	}
	events := &[]string{}
	r := &fakeRepo{memberRole: role, found: true, lockFound: true, members: members, events: events}
	us := &fakeUsers{res: getuser.Response{Found: true, Email: "baru@tim.id", Invitation: &getuser.Invitation{}}, repo: r, events: events}
	au := &fakeAudit{repo: r, events: events}
	return &env{repo: r, us: us, au: au, uc: NewUsecase(r, us, au, nil)}
}

func (e *env) calls() []string { return *e.repo.events }

func ctxFor(id, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Role: role, Email: "actor@tim.id"})
}

func in(target string) Input {
	return Input{ProjectID: pid, UserID: target, Meta: base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"}}
}

func noEffects(t *testing.T, e *env) {
	t.Helper()
	if e.repo.deleteCalls != 0 || len(e.au.evs) != 0 {
		t.Fatalf("delete=%d audit=%d", e.repo.deleteCalls, len(e.au.evs))
	}
}

func TestRevoke_OwnerRevokesPending(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); err != nil {
		t.Fatal(err)
	}
	if e.repo.deleteCalls != 1 || e.repo.deleted[0] != idB || len(e.au.evs) != 1 {
		t.Fatalf("%+v", e.repo)
	}
	ev := e.au.evs[0]
	if ev.Action != record.ActionProjectInvitationRevoked || ev.ProjectID != pid || ev.ProjectLabel != "Payments" ||
		ev.TargetType != "project_member" || ev.Target != idB || ev.Meta != in(idB).Meta {
		t.Fatalf("%+v", ev)
	}
	if len(ev.Args) != 3 || ev.Args["user_id"] != idB || ev.Args["user_label"] != "baru@tim.id" || ev.Args["role"] != "member" {
		t.Fatalf("%+v", ev.Args)
	}
	for _, k := range []string{"password", "temporary_password", "hash", "credentials"} {
		if _, ok := ev.Args[k]; ok {
			t.Fatalf("args memuat %s", k)
		}
	}
}

func TestRevoke_SuperAdminNonMember(t *testing.T) {
	e := newEnv(idS, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idS, "super_admin"), in(idB)); err != nil {
		t.Fatal(err)
	}
	if len(e.au.evs) != 1 {
		t.Fatal("audit")
	}
}

func TestRevoke_ExpiredPendingAllowed(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	e.us.res.Invitation = &getuser.Invitation{Expired: true}
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); err != nil {
		t.Fatal(err)
	}
}

func TestRevoke_NoIdentity(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"))
	err := e.uc.Execute(context.Background(), in(idB))
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeUnauthorized || len(e.calls()) != 0 {
		t.Fatalf("%v %v", err, e.calls())
	}
}

func TestRevoke_NonMemberActor404(t *testing.T) {
	for _, target := range []string{idB, "bukan-uuid"} {
		e := newEnv(idC, mem(idA, "owner"), mem(idB, "member"))
		e.repo.found = false
		err := e.uc.Execute(ctxFor(idC, "developer"), in(target))
		if !errors.Is(err, base.ErrProjectNotFound) || e.us.calls != 0 || e.repo.inTxCalls != 0 {
			t.Fatalf("%s: %v", target, err)
		}
	}
}

func TestRevoke_MemberForbidden(t *testing.T) {
	e := newEnv(idB, mem(idA, "owner"), mem(idB, "member"), mem(idC, "member"))
	err := e.uc.Execute(ctxFor(idB, "developer"), in(idC))
	if !errors.Is(err, base.ErrForbidden) || e.us.calls != 0 || e.repo.inTxCalls != 0 {
		t.Fatalf("%v", err)
	}
}

func TestRevoke_FindAccessError(t *testing.T) {
	boom := errors.New("boom")
	e := newEnv(idA, mem(idA, "owner"))
	e.repo.errFind = boom
	err := e.uc.Execute(ctxFor(idA, "developer"), in(idB))
	var ee *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &ee) {
		t.Fatal(err)
	}
}

func TestRevoke_TargetNotUUID(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"))
	err := e.uc.Execute(ctxFor(idA, "developer"), in("bukan-uuid"))
	if !errors.Is(err, base.ErrMemberNotFound) || e.us.calls != 0 {
		t.Fatal(err)
	}
}

func TestRevoke_UppercaseUUIDNormalized(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(strings.ToUpper(idB))); err != nil {
		t.Fatal(err)
	}
	if e.us.gotID != idB || e.repo.deleted[0] != idB {
		t.Fatalf("%s %v", e.us.gotID, e.repo.deleted)
	}
}

func TestRevoke_NotPending409(t *testing.T) {
	cases := map[string]func(*fakeUsers){
		"not found": func(u *fakeUsers) { u.res = getuser.Response{Found: false} },
		"accepted":  func(u *fakeUsers) { u.res.Invitation = nil },
	}
	for name, mod := range cases {
		e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
		mod(e.us)
		err := e.uc.Execute(ctxFor(idA, "developer"), in(idB))
		if !errors.Is(err, base.ErrInvitationNotPending) || e.repo.inTxCalls != 0 {
			t.Fatalf("%s: %v", name, err)
		}
		noEffects(t, e)
	}
}

func TestRevoke_GetUserErrorFailClosed(t *testing.T) {
	for _, boom := range []error{errors.New("boom"), bus.ErrNotRegistered} {
		e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
		e.us.err = boom
		err := e.uc.Execute(ctxFor(idA, "developer"), in(idB))
		var ee *errs.Error
		if !errors.Is(err, boom) || errors.As(err, &ee) || e.repo.inTxCalls != 0 {
			t.Fatalf("%v", err)
		}
		noEffects(t, e)
	}
}

func TestRevoke_TargetNotMember404(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"))
	err := e.uc.Execute(ctxFor(idA, "developer"), in(uuid.NewString()))
	if !errors.Is(err, base.ErrMemberNotFound) {
		t.Fatal(err)
	}
	noEffects(t, e)
}

func TestRevoke_ProjectGoneBetweenCheckAndLock(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	e.repo.lockFound = false
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatal(err)
	}
	noEffects(t, e)
}

func TestRevoke_ActorDemotedBetweenCheckAndLock(t *testing.T) {
	e := newEnv(idA, mem(idA, "member"), mem(idB, "member"), mem(idC, "owner"))
	e.repo.memberRole = "owner"
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); !errors.Is(err, base.ErrForbidden) {
		t.Fatal(err)
	}
	noEffects(t, e)
}

func TestRevoke_ActorRemovedBetweenCheckAndLock(t *testing.T) {
	e := newEnv(idA, mem(idC, "owner"), mem(idB, "member"))
	e.repo.memberRole = "owner"
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatal(err)
	}
	noEffects(t, e)
}

func TestRevoke_LastOwner(t *testing.T) {
	// aktor owner membatalkan owner terakhir (daftar terkunci hanya berisi target owner + aktor owner diturunkan tak relevan)
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "owner"))
	e.repo.members = []base.Member{mem(idB, "owner"), mem(idA, "member")}
	e.repo.memberRole = "owner"
	err := e.uc.Execute(ctxFor(idA, "developer"), in(idB))
	if !errors.Is(err, base.ErrForbidden) {
		t.Fatalf("aktor turun jadi member: %v", err)
	}
	// Super Admin boleh menghapus owner terakhir
	e = newEnv(idS, mem(idB, "owner"))
	if err := e.uc.Execute(ctxFor(idS, "super_admin"), in(idB)); err != nil {
		t.Fatal(err)
	}
	if e.au.evs[0].Args["role"] != "owner" {
		t.Fatal(e.au.evs[0].Args)
	}
}

func TestRevoke_LastOwnerByOwnerActor(t *testing.T) {
	// aktor owner = target? tidak mungkin; gunakan CheckRemove langsung lewat aktor owner yang juga satu-satunya owner dan target owner sama
	e := newEnv(idA, mem(idA, "owner"))
	e.us.res.Invitation = &getuser.Invitation{}
	err := e.uc.Execute(ctxFor(idA, "developer"), in(idA))
	if !errors.Is(err, base.ErrLastOwner) {
		t.Fatal(err)
	}
	noEffects(t, e)
}

func TestRevoke_OtherOwnerExists(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "owner"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); err != nil {
		t.Fatal(err)
	}
}

func TestRevoke_DeleteErrors(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	e.repo.errDelete = base.ErrMemberNotFound
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); !errors.Is(err, base.ErrMemberNotFound) {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	for name, mod := range map[string]func(*fakeRepo){
		"lockproject": func(r *fakeRepo) { r.errLock = boom },
		"lockmembers": func(r *fakeRepo) { r.errMembers = boom },
		"delete":      func(r *fakeRepo) { r.errDelete = boom },
	} {
		e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
		mod(e.repo)
		err := e.uc.Execute(ctxFor(idA, "developer"), in(idB))
		var ee *errs.Error
		if !errors.Is(err, boom) || errors.As(err, &ee) || len(e.au.evs) != 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestRevoke_Ordering(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); err != nil {
		t.Fatal(err)
	}
	want := "find,getuser,begin,lockproject,lockmembers,delete,commit,audit"
	if got := strings.Join(e.calls(), ","); got != want {
		t.Fatal(got)
	}
	if e.us.inTxAt || e.au.inTxAt {
		t.Fatal("bus dipanggil di dalam tx")
	}
}
