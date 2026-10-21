package changememberrole

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
)

var t0 = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

type fakeRepo struct {
	log        *[]string
	project    base.Project
	memberRole string
	found      bool
	lockOK     bool
	members    []base.Member
	updates    [][3]string
	failUpdate error
	failLockP  error
	failLockM  error
	inTx       int
}

func (f *fakeRepo) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	*f.log = append(*f.log, "FindAccess")
	return f.project, f.memberRole, f.found, nil
}

func (f *fakeRepo) InTx(_ context.Context, fn func(TxRepository) error) error {
	f.inTx++
	*f.log = append(*f.log, "tx-begin")
	err := fn(f)
	if err == nil {
		*f.log = append(*f.log, "tx-commit")
	}
	return err
}

func (f *fakeRepo) LockProject(context.Context, string) (base.Project, bool, error) {
	*f.log = append(*f.log, "LockProject")
	return f.project, f.lockOK, f.failLockP
}

func (f *fakeRepo) LockMembers(context.Context, string) ([]base.Member, error) {
	*f.log = append(*f.log, "LockMembers")
	return f.members, f.failLockM
}

func (f *fakeRepo) UpdateMemberRole(_ context.Context, p, u, r string) error {
	*f.log = append(*f.log, "UpdateMemberRole")
	if f.failUpdate != nil {
		return f.failUpdate
	}
	f.updates = append(f.updates, [3]string{p, u, r})
	return nil
}

type fakeUsers struct {
	log   *[]string
	resp  getuser.Response
	err   error
	calls int
}

func (u *fakeUsers) Get(context.Context, string) (getuser.Response, error) {
	u.calls++
	*u.log = append(*u.log, "users.Get")
	return u.resp, u.err
}

func (u *fakeUsers) FindDevelopers(context.Context, string, int) ([]finddevelopers.Developer, error) {
	return nil, nil
}

var _ users.Lookup = (*fakeUsers)(nil)

type fakeAudit struct {
	log    *[]string
	events []audit.Event
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	*a.log = append(*a.log, "audit")
	a.events = append(a.events, ev)
}

type env struct {
	uc   *Usecase
	repo *fakeRepo
	usr  *fakeUsers
	aud  *fakeAudit
	log  *[]string
	logs *bytes.Buffer
}

// newEnv: aktor u-1 dengan memberRole; anggota: u-1 owner, u-2 member.
func newEnv(memberRole string) env {
	log := &[]string{}
	r := &fakeRepo{
		log: log, memberRole: memberRole, found: true, lockOK: true,
		project: base.Project{ID: "p1", Name: "Proyek", CreatedAt: t0, UpdatedAt: t0},
		members: []base.Member{
			{ProjectID: "p1", UserID: "u-1", Role: "owner", AddedAt: t0},
			{ProjectID: "p1", UserID: "u-2", Role: "member", AddedAt: t0},
		},
	}
	us := &fakeUsers{log: log, resp: getuser.Response{Found: true, ID: "u-2", Name: "B", Email: "b@tim.id", Role: "developer", IsActive: true}}
	a := &fakeAudit{log: log}
	buf := &bytes.Buffer{}
	lg := slog.New(slog.NewTextHandler(buf, nil))
	return env{uc: NewUsecase(r, us, a, lg), repo: r, usr: us, aud: a, log: log, logs: buf}
}

func ctxFor(role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u-1", Email: "a@tim.id", Role: role})
}

func in(userID, role string) Input {
	return Input{ProjectID: "p1", UserID: userID, Role: role, Meta: base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"}}
}

func TestChangeRole_OwnerPromotesMember(t *testing.T) {
	e := newEnv("owner")
	out, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.repo.updates, [][3]string{{"p1", "u-2", "owner"}}) {
		t.Fatalf("updates: %v", e.repo.updates)
	}
	if out.Role != "owner" || len(e.aud.events) != 1 {
		t.Fatalf("out %+v events %d", out, len(e.aud.events))
	}
}

func TestChangeRole_OwnerDemotesOtherOwner(t *testing.T) {
	e := newEnv("owner")
	e.repo.members[1].Role = "owner"
	_, err := e.uc.Execute(ctxFor("developer"), in("u-2", "member"))
	if err != nil || len(e.repo.updates) != 1 || e.repo.updates[0][2] != "member" {
		t.Fatalf("err %v updates %v", err, e.repo.updates)
	}
}

func TestT03_AC3_OwnerDemotesSelfWhenAnotherOwnerExists(t *testing.T) {
	e := newEnv("owner")
	e.repo.members[1].Role = "owner"
	_, err := e.uc.Execute(ctxFor("developer"), in("u-1", "member"))
	if err != nil || len(e.repo.updates) != 1 {
		t.Fatalf("err %v updates %v", err, e.repo.updates)
	}
}

func TestT03_AC3_LastOwnerCannotDemoteSelf(t *testing.T) {
	e := newEnv("owner")
	_, err := e.uc.Execute(ctxFor("developer"), in("u-1", "member"))
	if !errors.Is(err, base.ErrLastOwner) {
		t.Fatalf("err: %v", err)
	}
	var ee *errs.Error
	if !errors.As(err, &ee) || detailsOf(ee)["reason"] != base.ReasonLastOwner {
		t.Fatalf("details: %v", err)
	}
	if len(e.repo.updates) != 0 || len(e.aud.events) != 0 {
		t.Fatal("tidak boleh menulis/audit")
	}
}

func TestT03_AC3_SuperAdminCanDemoteLastOwner(t *testing.T) {
	e := newEnv("")
	out, err := e.uc.Execute(ctxFor("super_admin"), in("u-1", "member"))
	if err != nil || out.Role != "member" || len(e.aud.events) != 1 || len(e.repo.updates) != 1 {
		t.Fatalf("err %v out %+v", err, out)
	}
}

func TestT03_AC7_SuperAdminPromotesMember(t *testing.T) {
	e := newEnv("")
	out, err := e.uc.Execute(ctxFor("super_admin"), in("u-2", "owner"))
	if err != nil || out.Role != "owner" {
		t.Fatalf("err %v out %+v", err, out)
	}
}

func TestT03_AC2_MemberForbidden(t *testing.T) {
	e := newEnv("member")
	_, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if !errors.Is(err, base.ErrForbidden) {
		t.Fatalf("err: %v", err)
	}
	if e.repo.inTx != 0 || e.usr.calls != 0 || len(e.aud.events) != 0 {
		t.Fatal("tx/users/audit tidak boleh dipanggil")
	}
}

func TestChangeRole_NonMemberActorGets404(t *testing.T) {
	e := newEnv("")
	_, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if !errors.Is(err, base.ErrProjectNotFound) || e.repo.inTx != 0 {
		t.Fatalf("err %v tx %d", err, e.repo.inTx)
	}
}

func TestChangeRole_ProjectNotFound(t *testing.T) {
	e := newEnv("owner")
	e.repo.found = false
	_, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if !errors.Is(err, base.ErrProjectNotFound) || e.repo.inTx != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestChangeRole_DeletedBetweenCheckAndLock(t *testing.T) {
	e := newEnv("owner")
	e.repo.lockOK = false
	_, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if !errors.Is(err, base.ErrProjectNotFound) || len(e.repo.updates) != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestChangeRole_TargetNotMember(t *testing.T) {
	for _, id := range []string{"u-9", "bukan-uuid"} {
		e := newEnv("owner")
		_, err := e.uc.Execute(ctxFor("developer"), in(id, "owner"))
		if !errors.Is(err, base.ErrMemberNotFound) {
			t.Fatalf("%s: %v", id, err)
		}
		if len(e.repo.updates) != 0 || len(e.aud.events) != 0 || e.usr.calls != 0 {
			t.Fatal("efek samping")
		}
	}
}

func TestChangeRole_NonMemberActorWithMissingTargetStill404Project(t *testing.T) {
	e := newEnv("")
	_, err := e.uc.Execute(ctxFor("developer"), in("u-9", "owner"))
	if !errors.Is(err, base.ErrProjectNotFound) || errors.Is(err, base.ErrMemberNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestChangeRole_InvalidRole400(t *testing.T) {
	for _, role := range []string{"admin", "OWNER", " owner"} {
		e := newEnv("owner")
		_, err := e.uc.Execute(ctxFor("developer"), in("u-2", role))
		var ee *errs.Error
		if !errors.As(err, &ee) || ee.Code != errs.CodeInvalid {
			t.Fatalf("%q: %v", role, err)
		}
		fields, _ := detailsOf(ee)["fields"].(base.FieldErrors)
		if fields["role"] != "role harus owner atau member" {
			t.Fatalf("%q fields: %v", role, ee.Details)
		}
		if len(e.repo.updates) != 0 || len(e.aud.events) != 0 {
			t.Fatal("efek samping")
		}
	}
}

func TestChangeRole_NoopSkipsWriteAndAudit(t *testing.T) {
	e := newEnv("owner")
	e.repo.members[1].Role = "owner"
	out, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if err != nil || out.Role != "owner" {
		t.Fatalf("err %v out %+v", err, out)
	}
	if len(e.repo.updates) != 0 || len(e.aud.events) != 0 || e.usr.calls != 1 {
		t.Fatalf("updates %v events %d users %d", e.repo.updates, len(e.aud.events), e.usr.calls)
	}
}

func TestChangeRole_NoopOnLastOwnerIsOK(t *testing.T) {
	e := newEnv("owner")
	out, err := e.uc.Execute(ctxFor("developer"), in("u-1", "owner"))
	if err != nil || out.Role != "owner" || len(e.repo.updates) != 0 {
		t.Fatalf("err %v out %+v", err, out)
	}
}

func TestChangeRole_RecomputesActorLevelInsideTx(t *testing.T) {
	e := newEnv("owner")
	e.repo.members[0].Role = "member" // aktor sudah diturunkan tx lain
	_, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if !errors.Is(err, base.ErrForbidden) || len(e.repo.updates) != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestChangeRole_ActorRemovedBeforeLock(t *testing.T) {
	e := newEnv("owner")
	e.repo.members = e.repo.members[1:]
	_, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestChangeRole_AuditPayload(t *testing.T) {
	e := newEnv("owner")
	if _, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner")); err != nil {
		t.Fatal(err)
	}
	if len(e.aud.events) != 1 {
		t.Fatalf("events %d", len(e.aud.events))
	}
	want := audit.Event{
		Action: record.ActionProjectMemberRoleChanged, ProjectID: "p1", ProjectLabel: "Proyek",
		TargetType: "project_member", Target: "u-2",
		Args: map[string]any{"user_id": "u-2", "user_label": "b@tim.id", "role_from": "member", "role_to": "owner"},
		Meta: base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"},
	}
	if !reflect.DeepEqual(e.aud.events[0], want) {
		t.Fatalf("got %+v", e.aud.events[0])
	}
}

func TestChangeRole_AuditAfterTxAndGetUser(t *testing.T) {
	e := newEnv("owner")
	if _, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner")); err != nil {
		t.Fatal(err)
	}
	want := []string{"FindAccess", "tx-begin", "LockProject", "LockMembers", "UpdateMemberRole", "tx-commit", "users.Get", "audit"}
	if !reflect.DeepEqual(*e.log, want) {
		t.Fatalf("urutan: %v", *e.log)
	}
}

func TestChangeRole_GetUserFailureIsSoft(t *testing.T) {
	e := newEnv("owner")
	e.usr.err = errors.New("bus down")
	out, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Found || out.Name != nil || out.Email != nil || out.IsActive || out.Role != "owner" {
		t.Fatalf("out %+v", out)
	}
	if e.aud.events[0].Args["user_label"] != "" {
		t.Fatalf("label: %v", e.aud.events[0].Args)
	}
	if !strings.Contains(e.logs.String(), "level=WARN") {
		t.Fatalf("log: %s", e.logs.String())
	}
}

func TestChangeRole_GetUserNotFound(t *testing.T) {
	e := newEnv("owner")
	e.usr.resp = getuser.Response{}
	out, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if err != nil || out.Found || e.aud.events[0].Args["user_label"] != "" {
		t.Fatalf("err %v out %+v", err, out)
	}
}

func TestChangeRole_TxErrorNoAudit(t *testing.T) {
	e := newEnv("owner")
	boom := errors.New("boom")
	e.repo.failUpdate = boom
	_, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if !errors.Is(err, boom) || len(e.aud.events) != 0 || e.usr.calls != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestChangeRole_LockErrors(t *testing.T) {
	boom := errors.New("boom")
	e := newEnv("owner")
	e.repo.failLockP = boom
	if _, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner")); !errors.Is(err, boom) || len(e.aud.events) != 0 {
		t.Fatalf("lock project: %v", err)
	}
	e = newEnv("owner")
	e.repo.failLockM = boom
	if _, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner")); !errors.Is(err, boom) || len(e.aud.events) != 0 {
		t.Fatalf("lock members: %v", err)
	}
}

func TestChangeRole_DTOKeepsAddedFields(t *testing.T) {
	e := newEnv("owner")
	by, label := "u-1", "a@tim.id"
	e.repo.members[1].AddedBy, e.repo.members[1].AddedByLabel = &by, &label
	out, err := e.uc.Execute(ctxFor("developer"), in("u-2", "owner"))
	if err != nil {
		t.Fatal(err)
	}
	if !out.AddedAt.Equal(t0) || out.AddedBy == nil || *out.AddedBy != "u-1" || out.AddedByLabel == nil || *out.AddedByLabel != "a@tim.id" {
		t.Fatalf("out %+v", out)
	}
}

func TestChangeRole_NoIdentity(t *testing.T) {
	e := newEnv("owner")
	_, err := e.uc.Execute(context.Background(), in("u-2", "owner"))
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeUnauthorized || len(*e.log) != 0 {
		t.Fatalf("err %v", err)
	}
}

func detailsOf(e *errs.Error) map[string]any {
	m, _ := e.Details.(map[string]any)
	return m
}
