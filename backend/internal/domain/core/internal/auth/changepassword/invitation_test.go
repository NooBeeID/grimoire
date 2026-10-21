package changepassword

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
)

func (e *env) setInv(expiresAt time.Time, accepted bool) {
	inv := base.Invitation{UserID: "u1", ExpiresAt: expiresAt}
	if accepted {
		t := time.Now().Add(-time.Hour)
		inv.AcceptedAt = &t
	}
	e.repo.inv, e.repo.hasInv = inv, true
}

func TestChangePassword_AcceptsPendingInvitation(t *testing.T) {
	e := setup(t)
	e.setInv(time.Now().Add(time.Hour), false)
	e.repo.accepted = true
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil {
		t.Fatal(err)
	}
	if len(e.rec.evs) != 2 || e.rec.evs[0].Action != "auth.password.changed" || e.rec.evs[1].Action != "user.invitation.accepted" {
		t.Fatalf("%+v", e.rec.evs)
	}
	ev := e.rec.evs[1]
	if ev.Target != "u1" || ev.TargetType != "user" || ev.Status != "ok" || len(ev.Args) != 1 || ev.Args["email"] != "budi@tim.id" {
		t.Fatalf("%+v", ev)
	}
}

func TestChangePassword_AcceptedNotRecordedTwice(t *testing.T) {
	e := setup(t)
	e.setInv(time.Now().Add(time.Hour), true)
	e.repo.accepted = false
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil {
		t.Fatal(err)
	}
	if len(e.rec.evs) != 1 || e.rec.evs[0].Action != "auth.password.changed" {
		t.Fatalf("%+v", e.rec.evs)
	}
}

func TestChangePassword_NoInvitation_Unchanged(t *testing.T) {
	e := setup(t)
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil || len(e.rec.evs) != 1 {
		t.Fatalf("%v %+v", err, e.rec.evs)
	}
}

func TestChangePassword_ExpiredInvitationRejected(t *testing.T) {
	e := setup(t)
	e.setInv(time.Now().Add(-time.Second), false)
	err := e.uc.Execute(e.ctx, in(oldPW, newPW))
	fe := fieldsOf(err)
	if len(fe) != 1 || fe["current_password"] != msgInvitationExpired {
		t.Fatalf("%v", err)
	}
	if e.repo.calls != 0 || e.h.verify.Load() != 0 || e.h.hash.Load() != 0 || len(e.rec.evs) != 0 {
		t.Fatal("tidak boleh ada efek samping")
	}
}

func TestChangePassword_ExactExpiryRejected(t *testing.T) {
	e := setup(t)
	fixed := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	e.uc.clock = func() time.Time { return fixed }
	e.setInv(fixed, false)
	if fieldsOf(e.uc.Execute(e.ctx, in(oldPW, newPW)))["current_password"] != msgInvitationExpired {
		t.Fatal("harus ditolak tepat di batas")
	}
}

func TestChangePassword_JustBeforeExpiryAccepted(t *testing.T) {
	e := setup(t)
	fixed := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	e.uc.clock = func() time.Time { return fixed }
	e.setInv(fixed.Add(time.Nanosecond), false)
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil {
		t.Fatal(err)
	}
}

func TestChangePassword_AcceptedInvitationNeverExpired(t *testing.T) {
	e := setup(t)
	e.setInv(time.Now().Add(-48*time.Hour), true)
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil {
		t.Fatal(err)
	}
}

func TestChangePassword_StaleNoAcceptedAudit(t *testing.T) {
	e := setup(t)
	e.setInv(time.Now().Add(time.Hour), false)
	e.repo.err = ErrStale
	err := e.uc.Execute(e.ctx, in(oldPW, newPW))
	var ee interface{ Error() string }
	if !errors.As(err, &ee) || !strings.Contains(err.Error(), "password berubah") || len(e.rec.evs) != 0 {
		t.Fatalf("%v %+v", err, e.rec.evs)
	}
}

func TestChangePassword_RepoErrorNoAcceptedAudit(t *testing.T) {
	e := setup(t)
	e.setInv(time.Now().Add(time.Hour), false)
	e.repo.err = errors.New("db mati")
	err := e.uc.Execute(e.ctx, in(oldPW, newPW))
	if !errors.Is(err, e.repo.err) || len(e.rec.evs) != 0 {
		t.Fatalf("%v %+v", err, e.rec.evs)
	}
}

func TestChangePassword_FindInvitationError(t *testing.T) {
	e := setup(t)
	e.repo.errInv = errors.New("db mati")
	err := e.uc.Execute(e.ctx, in(oldPW, newPW))
	if !errors.Is(err, e.repo.errInv) || e.repo.calls != 0 {
		t.Fatalf("%v", err)
	}
}

func TestChangePassword_AcceptedAuditNoSecrets(t *testing.T) {
	e := setup(t)
	e.setInv(time.Now().Add(time.Hour), false)
	e.repo.accepted = true
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(e.rec.evs[1])
	s := string(b)
	if strings.Contains(s, oldPW) || strings.Contains(s, newPW) || strings.Contains(s, e.repo.user.PasswordHash) || strings.Contains(s, e.repo.got.NewHash) {
		t.Fatal(s)
	}
}

func TestChangePassword_UsesSingleNow(t *testing.T) {
	e := setup(t)
	calls := 0
	base0 := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	e.uc.clock = func() time.Time { calls++; return base0.Add(time.Duration(calls) * time.Hour) }
	e.setInv(base0.Add(90*time.Minute), false)
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !e.repo.got.Now.Equal(base0.Add(time.Hour)) {
		t.Fatalf("calls=%d now=%v", calls, e.repo.got.Now)
	}
}
