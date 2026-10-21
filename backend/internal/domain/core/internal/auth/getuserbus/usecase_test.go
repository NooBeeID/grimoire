package getuserbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/contracts/core/auth/getuser"
)

type fakeRepo struct {
	user     base.User
	found    bool
	err      error
	got      string
	gotEmail string
	emailHit int
	inv      base.Invitation
	hasInv   bool
	invErr   error
}

func (f *fakeRepo) FindUserByID(_ context.Context, id string) (base.User, bool, error) {
	f.got = id
	return f.user, f.found, f.err
}

func (f *fakeRepo) FindUserByEmail(_ context.Context, email string) (base.User, bool, error) {
	f.gotEmail = email
	f.emailHit++
	return f.user, f.found, f.err
}

func (f *fakeRepo) FindInvitation(context.Context, string) (base.Invitation, bool, error) {
	return f.inv, f.hasInv, f.invErr
}

const uid = "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02"

var now = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return now }

func newUC(r *fakeRepo) *Usecase { return NewUsecase(r, fixedClock) }

func byID(id string) getuser.Request { return getuser.Request{UserID: id} }

func pendingInv() base.Invitation {
	sent := now.Add(-time.Hour)
	return base.Invitation{
		UserID: uid, CreatedAt: now.Add(-2 * time.Hour), IssuedAt: now.Add(-90 * time.Minute),
		ExpiresAt: now.Add(24 * time.Hour), SentAt: &sent, DeliveryStatus: base.DeliverySent, ResendCount: 2,
	}
}

func TestUsecase_Execute_Found(t *testing.T) {
	u := base.User{ID: uid, Name: "Sari", Email: "sari@tim.id", Role: base.RoleDeveloper, IsActive: true, PasswordHash: "x"}
	res, err := newUC(&fakeRepo{user: u, found: true}).Execute(context.Background(), byID(uid))
	want := getuser.Response{Found: true, ID: uid, Name: "Sari", Email: "sari@tim.id", Role: "developer", IsActive: true}
	if err != nil || res != want {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_NotFound(t *testing.T) {
	res, err := newUC(&fakeRepo{}).Execute(context.Background(), byID(uid))
	if err != nil || res != (getuser.Response{}) {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_InactiveUserStillFound(t *testing.T) {
	res, err := newUC(&fakeRepo{user: base.User{ID: uid, IsActive: false}, found: true}).Execute(context.Background(), byID(uid))
	if err != nil || !res.Found || res.IsActive {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_SuperAdminRole(t *testing.T) {
	res, _ := newUC(&fakeRepo{user: base.User{ID: uid, Role: base.RoleSuperAdmin}, found: true}).Execute(context.Background(), byID(uid))
	if res.Role != "super_admin" {
		t.Fatalf("%+v", res)
	}
}

func TestUsecase_Execute_RepoError(t *testing.T) {
	boom := errors.New("db down")
	res, err := newUC(&fakeRepo{err: boom}).Execute(context.Background(), byID(uid))
	if !errors.Is(err, boom) || res != (getuser.Response{}) {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_EmailRepoError(t *testing.T) {
	boom := errors.New("db down")
	_, err := newUC(&fakeRepo{err: boom}).Execute(context.Background(), getuser.Request{Email: "a@x.id"})
	if !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
}

func TestUsecase_Execute_InvitationRepoError(t *testing.T) {
	boom := errors.New("inv down")
	r := &fakeRepo{user: base.User{ID: uid}, found: true, invErr: boom}
	res, err := newUC(r).Execute(context.Background(), byID(uid))
	if !errors.Is(err, boom) || res.Found {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_TrimsID(t *testing.T) {
	r := &fakeRepo{}
	_, _ = newUC(r).Execute(context.Background(), byID(" "+uid+" "))
	if r.got != uid {
		t.Fatalf("got %q", r.got)
	}
}

func TestUsecase_Execute_EmptyID(t *testing.T) {
	r := &fakeRepo{}
	res, err := newUC(r).Execute(context.Background(), byID(""))
	if err != nil || res.Found || r.got != "" || r.emailHit != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_InvitationPending(t *testing.T) {
	r := &fakeRepo{user: base.User{ID: uid, IsActive: true}, found: true, inv: pendingInv(), hasInv: true}
	res, err := newUC(r).Execute(context.Background(), byID(uid))
	inv := res.Invitation
	if err != nil || inv == nil {
		t.Fatalf("%+v %v", res, err)
	}
	if inv.Expired || inv.DeliveryStatus != base.DeliverySent || inv.ResendCount != 2 ||
		!inv.IssuedAt.Equal(r.inv.IssuedAt) || !inv.InvitedAt.Equal(r.inv.CreatedAt) ||
		!inv.ExpiresAt.Equal(r.inv.ExpiresAt) || inv.SentAt == nil {
		t.Fatalf("%+v", inv)
	}
}

func TestUsecase_Execute_InvitationExpired(t *testing.T) {
	i := pendingInv()
	i.ExpiresAt = now.Add(-time.Minute)
	r := &fakeRepo{user: base.User{ID: uid}, found: true, inv: i, hasInv: true}
	res, err := newUC(r).Execute(context.Background(), byID(uid))
	if err != nil || res.Invitation == nil || !res.Invitation.Expired {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_InvitationAccepted(t *testing.T) {
	i := pendingInv()
	i.Accept(now.Add(-time.Minute))
	r := &fakeRepo{user: base.User{ID: uid}, found: true, inv: i, hasInv: true}
	res, err := newUC(r).Execute(context.Background(), byID(uid))
	if err != nil || !res.Found || res.Invitation != nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_ByEmailNormalized(t *testing.T) {
	r := &fakeRepo{user: base.User{ID: uid, Email: "baru@contoh.id"}, found: true}
	res, err := newUC(r).Execute(context.Background(), getuser.Request{Email: "  Baru@Contoh.ID "})
	if err != nil || !res.Found || r.gotEmail != "baru@contoh.id" {
		t.Fatalf("%+v %v email=%q", res, err, r.gotEmail)
	}
}

func TestUsecase_Execute_UserIDWinsOverEmail(t *testing.T) {
	r := &fakeRepo{user: base.User{ID: uid}, found: true}
	res, err := newUC(r).Execute(context.Background(), getuser.Request{UserID: uid, Email: "a@x.id"})
	if err != nil || !res.Found || r.emailHit != 0 || r.got != uid {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_InvalidEmail(t *testing.T) {
	for _, e := range []string{"", "   ", "bukan-email", "Nama <a@x.id>"} {
		r := &fakeRepo{user: base.User{ID: uid}, found: true}
		res, err := newUC(r).Execute(context.Background(), getuser.Request{Email: e})
		if err != nil || res.Found || r.emailHit != 0 || r.got != "" {
			t.Fatalf("%q: %+v %v", e, res, err)
		}
	}
}

func TestUsecase_Execute_EmailNotFound(t *testing.T) {
	res, err := newUC(&fakeRepo{}).Execute(context.Background(), getuser.Request{Email: "tidak@ada.id"})
	if err != nil || res.Found || res.Invitation != nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestNewUsecase_NilClockDefaults(t *testing.T) {
	r := &fakeRepo{user: base.User{ID: uid}, found: true, inv: pendingInv(), hasInv: true}
	res, err := NewUsecase(r, nil).Execute(context.Background(), byID(uid))
	if err != nil || res.Invitation == nil {
		t.Fatalf("%+v %v", res, err)
	}
}
