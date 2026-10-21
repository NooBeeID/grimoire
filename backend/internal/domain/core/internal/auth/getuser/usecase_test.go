package getuser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/errs"
)

type fakeRepo struct {
	user   base.User
	found  bool
	err    error
	gotID  string
	inv    base.Invitation
	hasInv bool
	invErr error
}

func (f *fakeRepo) FindInvitation(context.Context, string) (base.Invitation, bool, error) {
	return f.inv, f.hasInv, f.invErr
}

var fixedNow = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return fixedNow }

func (f *fakeRepo) FindUserByID(_ context.Context, id string) (base.User, bool, error) {
	f.gotID = id
	return f.user, f.found, f.err
}

const testHash = "$argon2id$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA"

func sampleUser() base.User {
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	by := "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d"
	lbl := "admin@tim.id"
	return base.User{
		ID: "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02", Email: "sari@tim.id", Name: "Sari",
		PasswordHash: testHash, Role: base.RoleDeveloper, IsActive: true,
		CreatedAt: now, CreatedBy: &by, CreatedByLabel: &lbl, UpdatedAt: now,
	}
}

func TestUsecase_Execute_Found(t *testing.T) {
	for _, withLogin := range []bool{false, true} {
		u := sampleUser()
		if withLogin {
			l := time.Date(2026, 10, 21, 9, 10, 0, 0, time.UTC)
			u.LastLoginAt = &l
		}
		got, err := NewUsecase(&fakeRepo{user: u, found: true}, fixedClock).Execute(context.Background(), u.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := u.ToResponse()
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		if string(a) != string(b) {
			t.Fatalf("got %s want %s", a, b)
		}
		if (got.LastLoginAt != nil) != withLogin {
			t.Fatalf("last_login_at=%v", got.LastLoginAt)
		}
	}
}

func TestUsecase_Execute_NotFound(t *testing.T) {
	_, err := NewUsecase(&fakeRepo{}, fixedClock).Execute(context.Background(), "x")
	if !errors.Is(err, base.ErrUserNotFound) {
		t.Fatalf("err=%v", err)
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeNotFound {
		t.Fatalf("err=%v", err)
	}
}

func TestUsecase_Execute_InactiveUserStillReturned(t *testing.T) {
	u := sampleUser()
	u.IsActive = false
	got, err := NewUsecase(&fakeRepo{user: u, found: true}, fixedClock).Execute(context.Background(), u.ID)
	if err != nil || got.IsActive {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestUsecase_Execute_RepoError(t *testing.T) {
	base0 := errors.New("db down")
	_, err := NewUsecase(&fakeRepo{err: base0}, fixedClock).Execute(context.Background(), "x")
	if !errors.Is(err, base0) {
		t.Fatalf("err=%v", err)
	}
	var e *errs.Error
	if errors.As(err, &e) {
		t.Fatalf("tidak boleh *errs.Error: %v", err)
	}
}

func TestUsecase_Execute_DTOHasNoHash(t *testing.T) {
	u := sampleUser()
	got, err := NewUsecase(&fakeRepo{user: u, found: true}, fixedClock).Execute(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "password_hash") || strings.Contains(string(b), testHash) {
		t.Fatalf("bocor: %s", b)
	}
}

func TestUsecase_Execute_InvitationPending(t *testing.T) {
	u := sampleUser()
	inv := base.Invitation{
		UserID: u.ID, CreatedAt: fixedNow.Add(-time.Hour), IssuedAt: fixedNow.Add(-time.Hour),
		ExpiresAt: fixedNow.Add(time.Hour), DeliveryStatus: base.DeliveryPending,
	}
	got, err := NewUsecase(&fakeRepo{user: u, found: true, inv: inv, hasInv: true}, fixedClock).Execute(context.Background(), u.ID)
	if err != nil || got.Invitation == nil || !got.Invitation.Pending || got.Invitation.Expired {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	inv.ExpiresAt = fixedNow
	got, _ = NewUsecase(&fakeRepo{user: u, found: true, inv: inv, hasInv: true}, fixedClock).Execute(context.Background(), u.ID)
	if got.Invitation == nil || !got.Invitation.Expired {
		t.Fatalf("got=%+v", got)
	}
}

func TestUsecase_Execute_NoInvitationOrAccepted(t *testing.T) {
	u := sampleUser()
	got, err := NewUsecase(&fakeRepo{user: u, found: true}, fixedClock).Execute(context.Background(), u.ID)
	if err != nil || got.Invitation != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"invitation":null`) {
		t.Fatalf("json=%s", b)
	}
	acc := base.Invitation{UserID: u.ID, ExpiresAt: fixedNow.Add(time.Hour)}
	acc.Accept(fixedNow)
	got, _ = NewUsecase(&fakeRepo{user: u, found: true, inv: acc, hasInv: true}, fixedClock).Execute(context.Background(), u.ID)
	if got.Invitation != nil {
		t.Fatalf("got=%+v", got)
	}
}

func TestUsecase_Execute_InvitationRepoErrorWrapped(t *testing.T) {
	boom := errors.New("inv down")
	u := sampleUser()
	_, err := NewUsecase(&fakeRepo{user: u, found: true, invErr: boom}, fixedClock).Execute(context.Background(), u.ID)
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}
