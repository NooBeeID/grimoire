package removemember

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/inviteuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
)

const pid = "p1"

var (
	idA = uuid.NewString()
	idB = uuid.NewString()
	idC = uuid.NewString()
	idS = uuid.NewString()
)

type fakeRepo struct {
	memberRole string // hasil pre-check
	found      bool
	lockFound  bool
	members    []base.Member

	errFind, errLock, errMembers, errDelete error

	inTxCalls   int
	deleteCalls int
	deleted     []string
	inTx        bool
	events      *[]string
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
	repo   *fakeRepo
	inTxAt bool
	events *[]string
}

func (u *fakeUsers) Get(_ context.Context, id string) (getuser.Response, error) {
	u.calls++
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

func (u *fakeUsers) FindDevelopers(context.Context, string, int) ([]finddevelopers.Developer, error) {
	return nil, nil
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
	logs *bytes.Buffer
}

// newEnv: aktor dengan memberRole pre-check = role pada daftar anggota.
func newEnv(actorID string, members ...base.Member) *env {
	role := ""
	for _, m := range members {
		if m.UserID == actorID {
			role = m.Role
		}
	}
	events := &[]string{}
	r := &fakeRepo{memberRole: role, found: true, lockFound: true, members: members, events: events}
	us := &fakeUsers{res: getuser.Response{Found: true, Email: "target@tim.id"}, repo: r, events: events}
	au := &fakeAudit{repo: r, events: events}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	return &env{repo: r, us: us, au: au, uc: NewUsecase(r, us, au, log), logs: &buf}
}

func (e *env) calls() []string { return *e.repo.events }

func ctxFor(id, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Role: role, Email: "actor@tim.id"})
}

func in(target string) Input {
	return Input{ProjectID: pid, UserID: target, Meta: base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"}}
}

func noSideEffects(t *testing.T, e *env) {
	t.Helper()
	if e.repo.deleteCalls != 0 || len(e.au.evs) != 0 || e.us.calls != 0 {
		t.Fatalf("delete=%d audit=%d getuser=%d", e.repo.deleteCalls, len(e.au.evs), e.us.calls)
	}
}

func TestRemove_OwnerRemovesMember(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); err != nil {
		t.Fatal(err)
	}
	if e.repo.deleteCalls != 1 || e.repo.deleted[0] != idB || len(e.au.evs) != 1 {
		t.Fatalf("%+v", e.repo)
	}
	a := e.au.evs[0].Args
	if a["user_id"] != idB || a["user_label"] != "target@tim.id" || a["role"] != "member" || a["self"] != false {
		t.Fatalf("%+v", a)
	}
}

func TestT03_AC2_MemberCannotRemoveOthers(t *testing.T) {
	e := newEnv(idB, mem(idA, "owner"), mem(idB, "member"), mem(idC, "member"))
	err := e.uc.Execute(ctxFor(idB, "developer"), in(idC))
	if !errors.Is(err, base.ErrForbidden) {
		t.Fatal(err)
	}
	noSideEffects(t, e)
}

func TestRemove_MemberLeavesSelf(t *testing.T) {
	e := newEnv(idB, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idB, "developer"), in(idB)); err != nil {
		t.Fatal(err)
	}
	if e.repo.deleted[0] != idB || e.au.evs[0].Args["self"] != true {
		t.Fatalf("%+v", e.au.evs)
	}
}

func TestRemove_OwnerLeavesWhenAnotherOwner(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idC, "owner"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idA)); err != nil {
		t.Fatal(err)
	}
	a := e.au.evs[0].Args
	if a["self"] != true || a["role"] != "owner" {
		t.Fatalf("%+v", a)
	}
}

func TestT03_AC3_LastOwnerCannotLeave(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	err := e.uc.Execute(ctxFor(idA, "developer"), in(idA))
	if !errors.Is(err, base.ErrLastOwner) {
		t.Fatal(err)
	}
	noSideEffects(t, e)
}

func TestT03_AC3_SuperAdminRemovesLastOwner(t *testing.T) {
	e := newEnv(idS, mem(idA, "owner"))
	if err := e.uc.Execute(ctxFor(idS, "super_admin"), in(idA)); err != nil {
		t.Fatal(err)
	}
	if e.repo.deleteCalls != 1 || e.au.evs[0].Args["self"] != false {
		t.Fatalf("%+v", e.au.evs)
	}
}

func TestRemove_OwnerRemovesOtherOwner(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idC, "owner"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idC)); err != nil {
		t.Fatal(err)
	}
	if e.repo.deleted[0] != idC {
		t.Fatal(e.repo.deleted)
	}
}

func TestRemove_SuperAdminRemovesMember(t *testing.T) {
	e := newEnv(idS, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idS, "super_admin"), in(idB)); err != nil {
		t.Fatal(err)
	}
	if e.au.evs[0].Args["self"] != false {
		t.Fatal(e.au.evs[0].Args)
	}
}

func TestRemove_SuperAdminCannotSelfLeave(t *testing.T) {
	e := newEnv(idS, mem(idA, "owner"))
	err := e.uc.Execute(ctxFor(idS, "super_admin"), in(idS))
	if !errors.Is(err, base.ErrMemberNotFound) {
		t.Fatal(err)
	}
	noSideEffects(t, e)
}

func TestRemove_TargetNotMember404(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"))
	err := e.uc.Execute(ctxFor(idA, "developer"), in(uuid.NewString()))
	if !errors.Is(err, base.ErrMemberNotFound) {
		t.Fatal(err)
	}
	noSideEffects(t, e)
}

func TestRemove_TargetNotUUID404(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"))
	err := e.uc.Execute(ctxFor(idA, "developer"), in("bukan-uuid"))
	if !errors.Is(err, base.ErrMemberNotFound) || e.repo.inTxCalls != 0 {
		t.Fatalf("%v %d", err, e.repo.inTxCalls)
	}
}

func TestRemove_TargetUppercaseUUID(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(strings.ToUpper(idB))); err != nil {
		t.Fatal(err)
	}
	if e.repo.deleted[0] != idB {
		t.Fatal(e.repo.deleted)
	}
}

func TestRemove_MemberRemovingNonMemberGets404(t *testing.T) {
	e := newEnv(idB, mem(idA, "owner"), mem(idB, "member"))
	err := e.uc.Execute(ctxFor(idB, "developer"), in(uuid.NewString()))
	if !errors.Is(err, base.ErrMemberNotFound) {
		t.Fatal(err)
	}
}

func TestRemove_NonMemberActorGets404(t *testing.T) {
	for _, target := range []string{idB, uuid.NewString(), "bukan-uuid"} {
		e := newEnv(idC, mem(idA, "owner"), mem(idB, "member"))
		err := e.uc.Execute(ctxFor(idC, "developer"), in(target))
		if !errors.Is(err, base.ErrProjectNotFound) {
			t.Fatalf("%s: %v", target, err)
		}
		if e.repo.inTxCalls != 0 {
			t.Fatal("tx dipanggil")
		}
		noSideEffects(t, e)
	}
}

func TestRemove_ProjectNotFound(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"))
	e.repo.found = false
	err := e.uc.Execute(ctxFor(idA, "developer"), in(idB))
	if !errors.Is(err, base.ErrProjectNotFound) || e.repo.inTxCalls != 0 {
		t.Fatalf("%v %d", err, e.repo.inTxCalls)
	}
}

func TestRemove_DeletedBetweenCheckAndLock(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	e.repo.lockFound = false
	err := e.uc.Execute(ctxFor(idA, "developer"), in(idB))
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatal(err)
	}
	noSideEffects(t, e)
}

func TestRemove_ActorRemovedBetweenCheckAndLock(t *testing.T) {
	e := newEnv(idB, mem(idA, "owner"), mem(idC, "member"))
	e.repo.memberRole = "member" // pre-check lolos, daftar terkunci tanpa B
	err := e.uc.Execute(ctxFor(idB, "developer"), in(idC))
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatal(err)
	}
	noSideEffects(t, e)
}

func TestRemove_ActorDemotedBetweenCheckAndLock(t *testing.T) {
	e := newEnv(idA, mem(idA, "member"), mem(idC, "member"), mem(idB, "owner"))
	e.repo.memberRole = "owner"
	err := e.uc.Execute(ctxFor(idA, "developer"), in(idC))
	if !errors.Is(err, base.ErrForbidden) {
		t.Fatal(err)
	}
	noSideEffects(t, e)
}

func TestRemove_LevelRecomputedFromLockedRows(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	e.repo.memberRole = "owner"
	err := e.uc.Execute(ctxFor(idA, "developer"), in(idA))
	if !errors.Is(err, base.ErrLastOwner) {
		t.Fatal(err)
	}
	noSideEffects(t, e)
}

func TestRemove_GetUserFailureDoesNotFail(t *testing.T) {
	cases := map[string]func(*fakeUsers){
		"error":          func(u *fakeUsers) { u.err = errors.New("boom") },
		"not registered": func(u *fakeUsers) { u.err = bus.ErrNotRegistered },
		"not found":      func(u *fakeUsers) { u.res.Found = false },
	}
	for name, mod := range cases {
		e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
		mod(e.us)
		if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(e.au.evs) != 1 || e.au.evs[0].Args["user_label"] != "" {
			t.Fatalf("%s: %+v", name, e.au.evs)
		}
		if e.us.err != nil && !strings.Contains(e.logs.String(), "level=WARN") {
			t.Fatalf("%s: log %s", name, e.logs.String())
		}
	}
}

func TestRemove_AuditAfterTx(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); err != nil {
		t.Fatal(err)
	}
	want := "find,begin,lockproject,lockmembers,delete,commit,getuser,audit"
	if got := strings.Join(e.calls(), ","); got != want {
		t.Fatalf("%s", got)
	}
	if e.us.inTxAt || e.au.inTxAt {
		t.Fatal("bus dipanggil di dalam tx")
	}
}

func TestRemove_AuditPayload(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); err != nil {
		t.Fatal(err)
	}
	ev := e.au.evs[0]
	if ev.Action != record.ActionProjectMemberRemoved || ev.ProjectID != pid || ev.ProjectLabel != "Payments" ||
		ev.TargetType != "project_member" || ev.Target != idB || ev.Meta != in(idB).Meta || len(ev.Args) != 4 {
		t.Fatalf("%+v", ev)
	}
}

func TestRemove_TxErrorNoAudit(t *testing.T) {
	boom := errors.New("boom")
	for name, mod := range map[string]func(*fakeRepo){
		"lockproject": func(r *fakeRepo) { r.errLock = boom },
		"lockmembers": func(r *fakeRepo) { r.errMembers = boom },
		"delete":      func(r *fakeRepo) { r.errDelete = boom },
	} {
		e := newEnv(idA, mem(idA, "owner"), mem(idB, "member"))
		mod(e.repo)
		err := e.uc.Execute(ctxFor(idA, "developer"), in(idB))
		if !errors.Is(err, boom) {
			t.Fatalf("%s: %v", name, err)
		}
		if len(e.au.evs) != 0 || e.us.calls != 0 {
			t.Fatalf("%s: efek samping", name)
		}
	}
}

func TestRemove_FindAccessError(t *testing.T) {
	boom := errors.New("boom")
	e := newEnv(idA, mem(idA, "owner"))
	e.repo.errFind = boom
	if err := e.uc.Execute(ctxFor(idA, "developer"), in(idB)); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestRemove_NoIdentity(t *testing.T) {
	e := newEnv(idA, mem(idA, "owner"))
	err := e.uc.Execute(context.Background(), in(idB))
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeUnauthorized || len(e.calls()) != 0 {
		t.Fatalf("%v %v", err, e.calls())
	}
}

// Stub T12: tidak dipakai feature ini.
func (u *fakeUsers) GetByEmail(context.Context, string) (getuser.Response, error) {
	return getuser.Response{}, nil
}

func (u *fakeUsers) Invite(context.Context, inviteuser.Request) (inviteuser.Response, error) {
	return inviteuser.Response{}, nil
}

func (u *fakeUsers) ResendInvitation(context.Context, resendinvitation.Request) (resendinvitation.Response, error) {
	return resendinvitation.Response{}, nil
}
