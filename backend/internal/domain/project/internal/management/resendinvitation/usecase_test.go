package resendinvitation

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/invitation"
	"grimoire/lib/contracts/core/auth/resendinvitation"
)

const secretPw = "Rahasia-Sementara-123"

var (
	ownerID  = uuid.NewString()
	memberID = uuid.NewString()
	target   = uuid.NewString()
)

type fakeRepo struct {
	found      bool
	memberRole string
	members    []base.Member
	errFind    error
	errList    error
	findCalls  int
	listCalls  int
}

func (f *fakeRepo) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	f.findCalls++
	return base.Project{ID: "p1", Name: "Payments"}, f.memberRole, f.found, f.errFind
}

func (f *fakeRepo) ListMembers(context.Context, string) ([]base.Member, error) {
	f.listCalls++
	return f.members, f.errList
}

type fakeUsers struct {
	get       getuser.Response
	getErr    error
	out       resendinvitation.Response
	resendErr error
	getCalls  int
	reqs      []resendinvitation.Request
}

func (f *fakeUsers) Get(context.Context, string) (getuser.Response, error) {
	f.getCalls++
	return f.get, f.getErr
}

func (f *fakeUsers) ResendInvitation(_ context.Context, r resendinvitation.Request) (resendinvitation.Response, error) {
	f.reqs = append(f.reqs, r)
	return f.out, f.resendErr
}

type env struct {
	repo  *fakeRepo
	users *fakeUsers
	uc    *Usecase
	logs  *bytes.Buffer
}

func newEnv() *env {
	e := &env{
		repo: &fakeRepo{found: true, memberRole: base.RoleOwner,
			members: []base.Member{{UserID: ownerID, Role: base.RoleOwner}, {UserID: target, Role: base.RoleMember}}},
		users: &fakeUsers{
			get: getuser.Response{Found: true, ID: target, Invitation: &getuser.Invitation{}},
			out: resendinvitation.Response{EmailSent: true, EmailStatus: invitation.EmailStatusSent,
				ExpiresAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 7*3600))},
		},
		logs: &bytes.Buffer{},
	}
	e.uc = NewUsecase(e.repo, e.users, slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return e
}

func ctxAs(id, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Role: role, Email: "owner@x.id"})
}

func ownerCtx() context.Context { return ctxAs(ownerID, "developer") }

func assertErr(t *testing.T, err error, want *errs.Error) {
	t.Helper()
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != want.Code || ee.Message != want.Message {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func (e *env) noLeak(t *testing.T) {
	t.Helper()
	if strings.Contains(e.logs.String(), secretPw) {
		t.Fatalf("log memuat password: %s", e.logs.String())
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	e := newEnv()
	_, err := e.uc.Execute(context.Background(), Input{ProjectID: "p1", UserID: target})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeUnauthorized {
		t.Fatalf("%v", err)
	}
	if e.repo.findCalls != 0 || len(e.users.reqs) != 0 {
		t.Fatal("tidak boleh ada panggilan")
	}
}

func TestExecute_NonMember(t *testing.T) {
	e := newEnv()
	e.repo.found = false
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	assertErr(t, err, base.ErrProjectNotFound)

	e = newEnv()
	e.repo.memberRole = ""
	_, err = e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	assertErr(t, err, base.ErrProjectNotFound)
	if e.users.getCalls != 0 || len(e.users.reqs) != 0 {
		t.Fatal("Users tidak boleh dipanggil")
	}
}

func TestExecute_RegularMemberForbidden(t *testing.T) {
	e := newEnv()
	e.repo.memberRole = base.RoleMember
	_, err := e.uc.Execute(ctxAs(memberID, "developer"), Input{ProjectID: "p1", UserID: target})
	assertErr(t, err, base.ErrForbidden)
	if e.users.getCalls != 0 || e.repo.listCalls != 0 {
		t.Fatal("tidak boleh lanjut")
	}
}

func TestExecute_OwnerSuccess(t *testing.T) {
	e := newEnv()
	res, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: strings.ToUpper(target)})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.users.reqs) != 1 {
		t.Fatalf("calls = %d", len(e.users.reqs))
	}
	want := resendinvitation.Request{UserID: target, InvitedBy: ownerID, InvitedByLabel: "owner@x.id", ProjectName: "Payments", Initial: false}
	if e.users.reqs[0] != want {
		t.Fatalf("%+v", e.users.reqs[0])
	}
	if !res.EmailSent || res.EmailStatus != "sent" || res.Credentials != nil {
		t.Fatalf("%+v", res)
	}
	if res.ExpiresAt.Location() != time.UTC {
		t.Fatal("ExpiresAt harus UTC")
	}
}

func TestExecute_SuperAdminNonMember(t *testing.T) {
	e := newEnv()
	e.repo.memberRole = ""
	admin := uuid.NewString()
	if _, err := e.uc.Execute(ctxAs(admin, "super_admin"), Input{ProjectID: "p1", UserID: target}); err != nil {
		t.Fatal(err)
	}
	if len(e.users.reqs) != 1 {
		t.Fatal("harus terkirim")
	}
}

func TestExecute_UserIDNotUUID(t *testing.T) {
	e := newEnv()
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: "bukan-uuid"})
	assertErr(t, err, base.ErrMemberNotFound)
	if e.repo.listCalls != 0 {
		t.Fatal("ListMembers tidak boleh dipanggil")
	}
}

func TestExecute_TargetNotProjectMember(t *testing.T) {
	e := newEnv()
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: uuid.NewString()})
	assertErr(t, err, base.ErrMemberNotFound)
	if e.users.getCalls != 0 || len(e.users.reqs) != 0 {
		t.Fatal("Users tidak boleh dipanggil")
	}
}

func TestExecute_GetNotFound(t *testing.T) {
	e := newEnv()
	e.users.get = getuser.Response{Found: false}
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	assertErr(t, err, base.ErrMemberNotFound)
	if len(e.users.reqs) != 0 {
		t.Fatal("tidak boleh kirim")
	}
}

func TestExecute_NotPending(t *testing.T) {
	e := newEnv()
	e.users.get = getuser.Response{Found: true}
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	assertErr(t, err, base.ErrInvitationNotPending)
	var ee *errs.Error
	errors.As(err, &ee)
	if d, _ := ee.Details.(map[string]any); d["reason"] != base.ReasonInvitationNotPending {
		t.Fatalf("%v", ee.Details)
	}
	if len(e.users.reqs) != 0 {
		t.Fatal("tidak boleh kirim")
	}
}

func TestExecute_ExpiredStillResends(t *testing.T) {
	e := newEnv()
	e.users.get = getuser.Response{Found: true, Invitation: &getuser.Invitation{Expired: true}}
	if _, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target}); err != nil {
		t.Fatal(err)
	}
	if len(e.users.reqs) != 1 {
		t.Fatal("harus lanjut")
	}
}

func TestExecute_RepoErrors(t *testing.T) {
	sentinel := errors.New("db mati")
	e := newEnv()
	e.repo.errFind = sentinel
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	var ee *errs.Error
	if !errors.Is(err, sentinel) || errors.As(err, &ee) {
		t.Fatalf("%v", err)
	}
	e = newEnv()
	e.repo.errList = sentinel
	_, err = e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	if !errors.Is(err, sentinel) || errors.As(err, &ee) {
		t.Fatalf("%v", err)
	}
}

func TestExecute_GetBusError(t *testing.T) {
	sentinel := errors.New("bus down")
	e := newEnv()
	e.users.getErr = sentinel
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	var ee *errs.Error
	if !errors.Is(err, sentinel) || errors.As(err, &ee) {
		t.Fatalf("%v", err)
	}
}

func TestExecute_AuthErrorsPassthrough(t *testing.T) {
	rl := errs.WithDetails(errs.CodeRateLimited, "terlalu sering", map[string]any{"retry_after_seconds": 60, "scope": "resend"})
	cases := []struct {
		name string
		err  error
	}{
		{"rate limited", rl},
		{"race not pending", base.ErrInvitationNotPending},
		{"inactive", errs.WithDetails(errs.CodeAlreadyRegistered, "nonaktif", map[string]any{"reason": "inactive"})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv()
			e.users.resendErr = errors.Join(errors.New("users: resend"), c.err)
			_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
			var got *errs.Error
			if !errors.As(err, &got) || got != c.err {
				t.Fatalf("err = %v, want %v", err, c.err)
			}
		})
	}
}

func TestExecute_AuthNotFoundMapped(t *testing.T) {
	e := newEnv()
	e.users.resendErr = errs.New(errs.CodeNotFound, "user tidak ada")
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	assertErr(t, err, base.ErrMemberNotFound)
}

func TestExecute_AuthNonErrsWrapped(t *testing.T) {
	sentinel := errors.New("boom")
	e := newEnv()
	e.users.resendErr = sentinel
	_, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	var ee *errs.Error
	if !errors.Is(err, sentinel) || errors.As(err, &ee) {
		t.Fatalf("%v", err)
	}
}

func TestExecute_CredentialsOnFailure(t *testing.T) {
	for _, status := range []string{invitation.EmailStatusFailed, invitation.EmailStatusNotConfigured} {
		e := newEnv()
		e.users.out = resendinvitation.Response{EmailSent: false, EmailStatus: status, ExpiresAt: time.Now(),
			Credentials: &invitation.Credentials{Email: "t@x.id", TemporaryPassword: secretPw, LoginURL: "https://app/login"}}
		res, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
		if err != nil {
			t.Fatal(err)
		}
		if res.EmailSent || res.EmailStatus != status || res.Credentials == nil ||
			*res.Credentials != (CredentialsResponse{Email: "t@x.id", TemporaryPassword: secretPw, LoginURL: "https://app/login"}) {
			t.Fatalf("%+v", res)
		}
		e.noLeak(t)
	}
}

func TestExecute_CredentialsDroppedWhenSent(t *testing.T) {
	e := newEnv()
	e.users.out = resendinvitation.Response{EmailSent: true, EmailStatus: "sent", ExpiresAt: time.Now(),
		Credentials: &invitation.Credentials{Email: "t@x.id", TemporaryPassword: secretPw, LoginURL: "u"}}
	res, err := e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	if err != nil {
		t.Fatal(err)
	}
	if res.Credentials != nil {
		t.Fatal("kredensial harus dibuang")
	}
	if !strings.Contains(e.logs.String(), "level=WARN") {
		t.Fatalf("harus ada warn: %s", e.logs.String())
	}
	e.noLeak(t)
}

func TestExecute_LogsNeverContainPassword(t *testing.T) {
	e := newEnv()
	e.users.resendErr = errors.New("gagal")
	e.users.out = resendinvitation.Response{Credentials: &invitation.Credentials{TemporaryPassword: secretPw}}
	_, _ = e.uc.Execute(ownerCtx(), Input{ProjectID: "p1", UserID: target})
	e.noLeak(t)
}
