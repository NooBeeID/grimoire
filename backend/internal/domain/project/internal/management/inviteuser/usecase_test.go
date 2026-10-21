package inviteuser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
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
	"grimoire/lib/contracts/core/auth/invitation"
	"grimoire/lib/contracts/core/auth/inviteuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
)

const (
	pid       = "p1"
	invitedID = "0192f5a0-cccc-7aaa-8bbb-00000000000c"
	fakePass  = "PASSWORD-RAHASIA-123"
)

var (
	now     = time.Date(2026, 10, 22, 9, 10, 0, 0, time.UTC)
	expires = time.Date(2026, 10, 29, 9, 10, 0, 0, time.UTC)
)

type env struct {
	inTx   bool
	events []string
}

func (e *env) add(s string) { e.events = append(e.events, s) }

type fakeRepo struct {
	e          *env
	p          base.Project
	memberRole string
	found      bool
	targetRole string
	lockOK     bool
	members    []base.Member
	inserted   bool
	inserts    []base.Member

	errFind, errLock, errMembers, errInsert error
	inTxCalls, insertCalls                  int
	findCalls                               int
}

func (f *fakeRepo) FindAccess(_ context.Context, _, uid string) (base.Project, string, bool, error) {
	f.findCalls++
	f.e.add("find")
	if uid == invitedID {
		return f.p, f.targetRole, true, nil
	}
	return f.p, f.memberRole, f.found, f.errFind
}

func (f *fakeRepo) InTx(_ context.Context, fn func(Tx) error) error {
	f.inTxCalls++
	f.e.add("intx")
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
	e *env

	byEmail    getuser.Response
	byEmailErr error
	invite     inviteuser.Response
	inviteErr  error
	resend     resendinvitation.Response
	resendErr  error
	get        getuser.Response
	getErr     error

	emailCalls, inviteCalls, resendCalls, getCalls int
	inviteReq                                      inviteuser.Request
	resendReq                                      resendinvitation.Request
	emailArg                                       string
	inTx                                           bool
}

func (u *fakeUsers) mark(name string) {
	u.inTx = u.inTx || u.e.inTx
	u.e.add(name)
}

func (u *fakeUsers) Get(context.Context, string) (getuser.Response, error) {
	u.getCalls++
	u.mark("get")
	return u.get, u.getErr
}
func (u *fakeUsers) FindDevelopers(context.Context, string, int) ([]finddevelopers.Developer, error) {
	return nil, nil
}
func (u *fakeUsers) GetByEmail(_ context.Context, email string) (getuser.Response, error) {
	u.emailCalls++
	u.emailArg = email
	u.mark("getByEmail")
	return u.byEmail, u.byEmailErr
}
func (u *fakeUsers) Invite(_ context.Context, in inviteuser.Request) (inviteuser.Response, error) {
	u.inviteCalls++
	u.inviteReq = in
	u.mark("invite")
	return u.invite, u.inviteErr
}
func (u *fakeUsers) ResendInvitation(_ context.Context, in resendinvitation.Request) (resendinvitation.Response, error) {
	u.resendCalls++
	u.resendReq = in
	u.mark("resend")
	return u.resend, u.resendErr
}

type fakeAudit struct {
	e    *env
	evs  []audit.Event
	inTx bool
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	a.inTx = a.inTx || a.e.inTx
	a.e.add("audit")
	a.evs = append(a.evs, ev)
}

type rig struct {
	uc  *Usecase
	r   *fakeRepo
	u   *fakeUsers
	a   *fakeAudit
	log *bytes.Buffer
}

func ctxFor(uid, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: uid, Role: role, Email: uid + "@x"})
}

func setup(actorRole string) *rig {
	e := &env{}
	r := &fakeRepo{
		e: e, p: base.Project{ID: pid, Name: "Payments"}, memberRole: actorRole, found: true, lockOK: true, inserted: true,
		members: []base.Member{{UserID: "a", Role: actorRole}},
	}
	u := &fakeUsers{
		e:      e,
		invite: inviteuser.Response{UserID: invitedID, Created: true, ExpiresAt: expires, EmailStatus: invitation.EmailStatusPending},
		resend: resendinvitation.Response{EmailSent: true, EmailStatus: invitation.EmailStatusSent, ExpiresAt: expires},
		get:    getuser.Response{Found: true, ID: invitedID, Name: "Budi", Email: "baru@contoh.id", Role: "developer", IsActive: true},
	}
	a := &fakeAudit{e: e}
	buf := &bytes.Buffer{}
	lg := slog.New(slog.NewTextHandler(buf, nil))
	return &rig{uc: NewUsecase(r, u, a, func() time.Time { return now }, lg), r: r, u: u, a: a, log: buf}
}

func req(role string) InviteRequest {
	return InviteRequest{Email: "baru@contoh.id", Role: role, Name: "Budi"}
}

func (g *rig) run(role string) (InviteResponse, error) {
	return g.uc.Execute(ctxFor("a", "developer"), pid, req(role), base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"})
}

func (g *rig) noBusOrSideEffects(t *testing.T) {
	t.Helper()
	if g.u.inviteCalls != 0 || g.u.resendCalls != 0 || g.u.getCalls != 0 || g.r.insertCalls != 0 || len(g.a.evs) != 0 {
		t.Fatalf("invite=%d resend=%d get=%d insert=%d audit=%d", g.u.inviteCalls, g.u.resendCalls, g.u.getCalls, g.r.insertCalls, len(g.a.evs))
	}
}

func TestExecute_Success_NewAccount(t *testing.T) {
	g := setup("owner")
	res, err := g.run("")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"find", "getByEmail", "invite", "intx", "resend", "get", "audit"}
	if !reflect.DeepEqual(g.r.e.events, want) {
		t.Fatalf("events %v", g.r.e.events)
	}
	if g.u.inTx || g.a.inTx {
		t.Fatal("bus/audit dipanggil di dalam tx")
	}
	if g.u.emailArg != "baru@contoh.id" {
		t.Fatal(g.u.emailArg)
	}
	wantInvite := inviteuser.Request{Email: "baru@contoh.id", Name: "Budi", InvitedBy: "a", InvitedByLabel: "a@x", ProjectName: "Payments", Deliver: false}
	if g.u.inviteReq != wantInvite {
		t.Fatalf("%+v", g.u.inviteReq)
	}
	wantResend := resendinvitation.Request{UserID: invitedID, InvitedBy: "a", InvitedByLabel: "a@x", ProjectName: "Payments", Initial: true}
	if g.u.resendReq != wantResend {
		t.Fatalf("%+v", g.u.resendReq)
	}
	m := g.r.inserts[0]
	if m.Role != base.RoleMember || m.UserID != invitedID || *m.AddedBy != "a" || *m.AddedByLabel != "a@x" || !m.AddedAt.Equal(now) {
		t.Fatalf("%+v", m)
	}
	if !res.EmailSent || res.EmailStatus != "sent" || res.Credentials != nil || !res.Created || !res.ExpiresAt.Equal(expires) {
		t.Fatalf("%+v", res)
	}
	if !res.Member.InvitationPending || res.Member.InvitationExpiresAt == nil || !res.Member.InvitationExpiresAt.Equal(expires) ||
		res.Member.UserID != invitedID || *res.Member.Name != "Budi" || res.Member.Role != "member" {
		t.Fatalf("%+v", res.Member)
	}
}

func TestT12_AC1_NoPasswordLeaks(t *testing.T) {
	g := setup("owner")
	g.u.resend.Credentials = &invitation.Credentials{Email: "baru@contoh.id", TemporaryPassword: fakePass, LoginURL: "https://x/login"}
	res, err := g.run("")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	s := string(b)
	if strings.Contains(s, "temporary_password") || strings.Contains(s, "credentials") || strings.Contains(s, fakePass) {
		t.Fatalf("bocor: %s", s)
	}
	for _, ev := range g.a.evs {
		if strings.Contains(strings.ToLower(jsonOf(ev.Args)), "pass") || strings.Contains(jsonOf(ev.Args), fakePass) {
			t.Fatalf("audit bocor: %v", ev.Args)
		}
	}
	if strings.Contains(g.log.String(), fakePass) {
		t.Fatal("log bocor")
	}
}

func jsonOf(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestT12_AC4_AlreadyMember(t *testing.T) {
	g := setup("owner")
	g.u.byEmail = getuser.Response{Found: true, ID: invitedID, Email: "baru@contoh.id", Role: "developer", IsActive: true}
	g.r.targetRole = "member"
	_, err := g.run("")
	if !errors.Is(err, base.ErrAlreadyMember) {
		t.Fatal(err)
	}
	g.noBusOrSideEffects(t)
	if g.r.inTxCalls != 0 {
		t.Fatal("intx dipanggil")
	}
}

func TestT12_AC4_AlreadyRegistered(t *testing.T) {
	for _, reason := range []string{"registered", "inactive", "not_developer"} {
		g := setup("owner")
		g.u.inviteErr = errs.WithDetails(errs.CodeAlreadyRegistered, "email sudah terdaftar", map[string]any{"reason": reason})
		_, err := g.run("")
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeAlreadyRegistered {
			t.Fatalf("%v", err)
		}
		if d, _ := e.Details.(map[string]any); d["reason"] != reason {
			t.Fatalf("%v", e.Details)
		}
		if g.r.inTxCalls != 0 || g.u.resendCalls != 0 || len(g.a.evs) != 0 {
			t.Fatal("efek samping")
		}
	}
}

func TestT12_AC4_PendingAccountReused(t *testing.T) {
	g := setup("owner")
	g.u.byEmail = getuser.Response{Found: true, ID: invitedID, Email: "baru@contoh.id", Role: "developer", IsActive: true}
	g.u.invite.Created = false
	res, err := g.run("")
	if err != nil {
		t.Fatal(err)
	}
	if res.Created || g.u.resendReq.Initial || g.u.inviteCalls != 1 || g.r.insertCalls != 1 {
		t.Fatalf("%+v %+v", res, g.u.resendReq)
	}
}

func TestExecute_InviteErrorsPassThrough(t *testing.T) {
	cases := []error{
		errs.WithDetails(errs.CodeRateLimited, "x", map[string]any{"scope": "invite", "retry_after_seconds": 10}),
		base.InvalidFields(base.FieldErrors{"email": "format email tidak valid"}),
	}
	for _, ie := range cases {
		g := setup("owner")
		g.u.inviteErr = ie
		_, err := g.run("")
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != ie.(*errs.Error).Code {
			t.Fatalf("%v", err)
		}
		if g.r.inTxCalls != 0 || g.r.insertCalls != 0 || len(g.a.evs) != 0 {
			t.Fatal("efek samping")
		}
	}
}

func TestT12_AC5_MemberForbidden(t *testing.T) {
	g := setup("member")
	_, err := g.run("")
	if !errors.Is(err, base.ErrForbidden) {
		t.Fatal(err)
	}
	g.noBusOrSideEffects(t)
	if g.u.emailCalls != 0 || g.r.inTxCalls != 0 {
		t.Fatal("bus/intx dipanggil")
	}
}

func TestT12_AC5_NonMemberAndMissingProject(t *testing.T) {
	g := setup("")
	_, err := g.run("")
	if !errors.Is(err, base.ErrProjectNotFound) || g.u.emailCalls != 0 {
		t.Fatalf("%v %d", err, g.u.emailCalls)
	}
	g = setup("")
	g.r.found = false
	_, err = g.run("")
	if !errors.Is(err, base.ErrProjectNotFound) || g.u.emailCalls != 0 {
		t.Fatalf("%v %d", err, g.u.emailCalls)
	}
}

func TestExecute_SuperAdminNonMemberAllowed(t *testing.T) {
	g := setup("")
	g.r.members = nil
	_, err := g.uc.Execute(ctxFor("admin", "super_admin"), pid, req("owner"), base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if g.r.inserts[0].Role != base.RoleOwner || *g.r.inserts[0].AddedBy != "admin" {
		t.Fatalf("%+v", g.r.inserts[0])
	}
}

func TestExecute_Unauthenticated(t *testing.T) {
	g := setup("owner")
	_, err := g.uc.Execute(context.Background(), pid, req(""), base.RequestMeta{})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatal(err)
	}
}

func TestExecute_RoleValidation(t *testing.T) {
	for _, role := range []string{"admin", "OWNER"} {
		g := setup("owner")
		_, err := g.run(role)
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
			t.Fatalf("%s: %v", role, err)
		}
		if g.u.emailCalls != 0 {
			t.Fatal("bus dipanggil sebelum validasi role")
		}
	}
	g := setup("owner")
	if _, err := g.run("owner"); err != nil || g.r.inserts[0].Role != base.RoleOwner {
		t.Fatal(err)
	}
}

func TestT12_AC9_SendFailsFallbackCredentials(t *testing.T) {
	for _, st := range []string{invitation.EmailStatusFailed, invitation.EmailStatusNotConfigured} {
		g := setup("owner")
		g.u.resend = resendinvitation.Response{
			EmailSent: false, EmailStatus: st, ExpiresAt: expires,
			Credentials: &invitation.Credentials{Email: "baru@contoh.id", TemporaryPassword: fakePass, LoginURL: "https://x/login"},
		}
		res, err := g.run("")
		if err != nil {
			t.Fatal(err)
		}
		if res.EmailSent || res.EmailStatus != st || res.Credentials == nil ||
			res.Credentials.TemporaryPassword != fakePass || res.Credentials.Email != "baru@contoh.id" || res.Credentials.LoginURL != "https://x/login" {
			t.Fatalf("%+v", res)
		}
		if len(g.a.evs) != 1 || strings.Contains(jsonOf(g.a.evs[0].Args), fakePass) || strings.Contains(g.log.String(), fakePass) {
			t.Fatal("audit/log bocor atau jumlah salah")
		}
	}
}

func TestT12_AC9_ResendErrorDegrades(t *testing.T) {
	errsList := []error{
		errors.New("auth down"),
		bus.ErrNotRegistered,
		errs.WithDetails(errs.CodeRateLimited, "x", map[string]any{"scope": "resend"}),
		errs.WithDetails(errs.CodeConflict, "x", map[string]any{"reason": "INVITATION_NOT_PENDING"}),
	}
	for _, re := range errsList {
		g := setup("owner")
		g.u.resendErr = re
		res, err := g.run("")
		if err != nil {
			t.Fatalf("%v: %v", re, err)
		}
		if res.EmailSent || res.EmailStatus != invitation.EmailStatusFailed || res.Credentials != nil || !res.ExpiresAt.Equal(expires) {
			t.Fatalf("%+v", res)
		}
		if g.r.insertCalls != 1 || len(g.a.evs) != 1 {
			t.Fatalf("insert=%d audit=%d", g.r.insertCalls, len(g.a.evs))
		}
		lg := g.log.String()
		if !strings.Contains(lg, "project_id=p1") || !strings.Contains(lg, "user_id="+invitedID) || !strings.Contains(lg, "level=ERROR") ||
			strings.Contains(lg, "baru@contoh.id") {
			t.Fatalf("log: %s", lg)
		}
	}
}

func TestT12_AC10_NoCredentialsOnSuccess(t *testing.T) {
	g := setup("owner")
	g.u.resend.Credentials = &invitation.Credentials{Email: "baru@contoh.id", TemporaryPassword: fakePass, LoginURL: "u"}
	res, err := g.run("")
	if err != nil || res.Credentials != nil || !res.EmailSent {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestExecute_GetFailsFallsBack(t *testing.T) {
	for _, mode := range []string{"err", "notfound"} {
		g := setup("owner")
		if mode == "err" {
			g.u.getErr = errors.New("down")
		} else {
			g.u.get = getuser.Response{}
		}
		in := req("")
		in.Name = ""
		res, err := g.uc.Execute(ctxFor("a", "developer"), pid, in, base.RequestMeta{})
		if err != nil {
			t.Fatal(err)
		}
		m := res.Member
		if !m.Found || m.UserID != invitedID || *m.Email != "baru@contoh.id" || m.Name != nil || !m.IsActive || !m.InvitationPending {
			t.Fatalf("%+v", m)
		}
		if len(g.a.evs) != 1 || !strings.Contains(g.log.String(), "level=WARN") {
			t.Fatalf("audit=%d log=%s", len(g.a.evs), g.log.String())
		}
	}
}

func TestExecute_TxFailures(t *testing.T) {
	t.Run("project hilang", func(t *testing.T) {
		g := setup("owner")
		g.r.lockOK = false
		_, err := g.run("")
		if !errors.Is(err, base.ErrProjectNotFound) || g.u.inviteCalls != 1 || g.u.resendCalls != 0 || len(g.a.evs) != 0 {
			t.Fatalf("%v", err)
		}
	})
	t.Run("aktor didemosi", func(t *testing.T) {
		g := setup("owner")
		g.r.members = []base.Member{{UserID: "a", Role: "member"}}
		_, err := g.run("")
		if !errors.Is(err, base.ErrForbidden) || g.r.insertCalls != 0 || g.u.resendCalls != 0 || len(g.a.evs) != 0 {
			t.Fatalf("%v", err)
		}
	})
	t.Run("target sudah terkunci", func(t *testing.T) {
		g := setup("owner")
		g.r.members = append(g.r.members, base.Member{UserID: invitedID, Role: "member"})
		_, err := g.run("")
		if !errors.Is(err, base.ErrAlreadyMember) || g.r.insertCalls != 0 || g.u.resendCalls != 0 || len(g.a.evs) != 0 {
			t.Fatalf("%v", err)
		}
	})
	t.Run("insert 0 baris", func(t *testing.T) {
		g := setup("owner")
		g.r.inserted = false
		_, err := g.run("")
		if !errors.Is(err, base.ErrAlreadyMember) || g.u.resendCalls != 0 || len(g.a.evs) != 0 {
			t.Fatalf("%v", err)
		}
	})
	for name, set := range map[string]func(*fakeRepo){
		"lock members": func(r *fakeRepo) { r.errMembers = errors.New("boom") },
		"insert":       func(r *fakeRepo) { r.errInsert = errors.New("boom") },
		"lock project": func(r *fakeRepo) { r.errLock = errors.New("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			g := setup("owner")
			set(g.r)
			_, err := g.run("")
			var e *errs.Error
			if err == nil || errors.As(err, &e) || g.u.resendCalls != 0 || len(g.a.evs) != 0 {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestExecute_GetByEmailError(t *testing.T) {
	for _, ge := range []error{errors.New("down"), bus.ErrNotRegistered} {
		g := setup("owner")
		g.u.byEmailErr = ge
		_, err := g.run("")
		var e *errs.Error
		if err == nil || errors.As(err, &e) || !errors.Is(err, ge) || g.u.inviteCalls != 0 {
			t.Fatalf("%v", err)
		}
	}
}

func TestExecute_FindAccessError(t *testing.T) {
	g := setup("owner")
	g.r.errFind = errors.New("db")
	_, err := g.run("")
	if err == nil || g.u.emailCalls != 0 {
		t.Fatal(err)
	}
}

func TestExecute_UserIDFromAuthNotClient(t *testing.T) {
	g := setup("owner")
	if _, err := g.run(""); err != nil {
		t.Fatal(err)
	}
	if g.r.inserts[0].UserID != invitedID || g.a.evs[0].Target != invitedID {
		t.Fatal("user_id harus dari auth")
	}
}

func TestExecute_AuditEvent(t *testing.T) {
	g := setup("owner")
	meta := base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"}
	if _, err := g.uc.Execute(ctxFor("a", "developer"), pid, req(""), meta); err != nil {
		t.Fatal(err)
	}
	if len(g.a.evs) != 1 {
		t.Fatal(len(g.a.evs))
	}
	ev := g.a.evs[0]
	if ev.Action != record.ActionProjectMemberInvited || ev.ProjectID != pid || ev.ProjectLabel != "Payments" ||
		ev.TargetType != "project_member" || ev.Target != invitedID || ev.Meta != meta {
		t.Fatalf("%+v", ev)
	}
	want := map[string]any{"user_id": invitedID, "user_label": "baru@contoh.id", "role": "member", "created": true}
	if !reflect.DeepEqual(ev.Args, want) {
		t.Fatalf("%+v", ev.Args)
	}
}
