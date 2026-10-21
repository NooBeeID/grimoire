package login

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/errs"
)

func (f *fixture) setInv(expiresAt time.Time, accepted bool) {
	inv := base.Invitation{UserID: "u-1", ExpiresAt: expiresAt}
	if accepted {
		t := testNow.Add(-time.Hour)
		inv.AcceptedAt = &t
	}
	f.repo.inv, f.repo.hasInv = inv, true
}

func TestLogin_ExpiredInvitation_SameAsWrongPassword(t *testing.T) {
	f := newFixture(t)
	f.setInv(testNow, false)
	_, err := f.run("budi@tim.id", goodPW)
	if !errors.Is(err, base.ErrInvalidCreds) || err != base.ErrInvalidCreds {
		t.Fatalf("%v", err)
	}
	if len(f.repo.completed) != 0 || f.repo.updates != 1 || len(f.rec.events) != 1 {
		t.Fatalf("completed=%d updates=%d events=%+v", len(f.repo.completed), f.repo.updates, f.rec.events)
	}
	ev := f.rec.events[0]
	if ev.Action != "auth.login.failed" || ev.Args["reason"] != "invitation_expired" || ev.Target != "u-1" {
		t.Fatalf("%+v", ev)
	}
}

func TestLogin_ExpiredInvitation_ErrorEqualsWrongPassword(t *testing.T) {
	f := newFixture(t)
	f.setInv(testNow, false)
	_, e1 := f.run("budi@tim.id", goodPW)
	g := newFixture(t)
	_, e2 := g.run("budi@tim.id", "salah-salah-1")
	a, b := asErr(t, e1), asErr(t, e2)
	if !errors.Is(e1, e2) || a.Code != b.Code || a.Message != b.Message || a.Details != b.Details {
		t.Fatalf("%v vs %v", e1, e2)
	}
}

func TestLogin_ExpiredInvitation_CountsTowardLockout(t *testing.T) {
	run := func(pw string, setup func(*fixture)) (codes []errs.Code, locked *time.Time) {
		f := newFixture(t)
		setup(f)
		for i := 0; i < 5; i++ {
			_, err := f.run("budi@tim.id", pw)
			codes = append(codes, asErr(t, err).Code)
		}
		locked = f.repo.throttle.LockedUntil
		_, err := f.run("budi@tim.id", pw)
		codes = append(codes, asErr(t, err).Code)
		last := f.rec.events[len(f.rec.events)-1]
		if last.Args["reason"] != "locked" {
			t.Fatalf("%+v", last)
		}
		return
	}
	c1, l1 := run(goodPW, func(f *fixture) { f.setInv(testNow, false) })
	c2, l2 := run("salah-salah-1", func(*fixture) {})
	if fmt.Sprint(c1) != fmt.Sprint(c2) || l1 == nil || l2 == nil || !l1.Equal(*l2) {
		t.Fatalf("%v %v / %v %v", c1, l1, c2, l2)
	}
	if c1[5] != base.CodeAccountLocked {
		t.Fatalf("%v", c1)
	}
}

func TestLogin_PendingNotExpired_Succeeds(t *testing.T) {
	f := newFixture(t)
	f.setInv(testNow.Add(time.Nanosecond), false)
	res, err := f.run("budi@tim.id", goodPW)
	if err != nil || !res.User.MustChangePassword || len(f.repo.completed) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	if len(f.rec.events) != 1 || f.rec.events[0].Action != "auth.login.success" {
		t.Fatalf("%+v", f.rec.events)
	}
}

func TestLogin_AcceptedInvitation_Succeeds(t *testing.T) {
	f := newFixture(t)
	f.setInv(testNow.Add(-24*time.Hour), true)
	if _, err := f.run("budi@tim.id", goodPW); err != nil {
		t.Fatal(err)
	}
}

func TestLogin_NoInvitation_Unchanged(t *testing.T) {
	f := newFixture(t)
	if _, err := f.run("budi@tim.id", goodPW); err != nil || f.repo.invFinds != 1 {
		t.Fatalf("%v finds=%d", err, f.repo.invFinds)
	}
}

func TestLogin_ExpiredButWrongPassword_NoInvitationLookup(t *testing.T) {
	f := newFixture(t)
	f.setInv(testNow.Add(-time.Hour), false)
	_, _ = f.run("budi@tim.id", "salah-salah-1")
	if f.repo.invFinds != 0 || f.rec.events[0].Args["reason"] != "invalid_credentials" {
		t.Fatalf("finds=%d %+v", f.repo.invFinds, f.rec.events)
	}
}

func TestLogin_UnknownEmail_NoInvitationLookup(t *testing.T) {
	f := newFixture(t)
	f.repo.found = false
	_, _ = f.run("tidakada@tim.id", goodPW)
	if f.repo.invFinds != 0 || len(f.hasher.hashes) != 1 || f.hasher.hashes[0] != f.dummy {
		t.Fatalf("finds=%d", f.repo.invFinds)
	}
}

func TestLogin_InactiveAndExpired_ReasonInactive(t *testing.T) {
	f := newFixture(t)
	f.repo.user.IsActive = false
	f.setInv(testNow.Add(-time.Hour), false)
	_, err := f.run("budi@tim.id", goodPW)
	if err != base.ErrInvalidCreds || f.repo.invFinds != 0 || f.rec.events[0].Args["reason"] != "inactive" {
		t.Fatalf("%v finds=%d %+v", err, f.repo.invFinds, f.rec.events)
	}
}

func TestLogin_FindInvitationError_FailsClosed(t *testing.T) {
	f := newFixture(t)
	f.repo.errInv = errors.New("db mati")
	_, err := f.run("budi@tim.id", goodPW)
	if err == nil || err == base.ErrInvalidCreds || !errors.Is(err, f.repo.errInv) {
		t.Fatalf("%v", err)
	}
	if len(f.repo.completed) != 0 || f.repo.updates != 0 {
		t.Fatal("tidak boleh ada sesi/throttle")
	}
}

func TestLogin_ExpiredAudit_NoSecrets(t *testing.T) {
	f := newFixture(t)
	f.setInv(testNow, false)
	_, _ = f.run("budi@tim.id", goodPW)
	b, _ := json.Marshal(f.rec.events[0])
	if strings.Contains(string(b), goodPW) || strings.Contains(string(b), f.repo.user.PasswordHash) {
		t.Fatalf("%s", b)
	}
}
