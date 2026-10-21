//go:build integration

package resendinvitation

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
	"grimoire/migrations"
)

func setupRepo(t *testing.T) (*db.DB, *Repo, *store.Store) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, NewRepo(d), store.New(d)
}

func seedInvitee(t *testing.T, d *db.DB, s *store.Store, active bool) (string, base.Invitation) {
	t.Helper()
	var inviter, id string
	if err := d.GetContext(context.Background(), &inviter, `INSERT INTO users (email, name, password_hash, role, is_active)
		VALUES ('inv-'||gen_random_uuid()::text||'@x.id', 'Admin', 'h', 'super_admin', true) RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	if err := d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
		VALUES ('new-'||gen_random_uuid()::text||'@x.id', 'Baru', 'old', 'developer', $1, true) RETURNING id::text`, active); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	inv := base.NewInvitation(id, inviter, "Admin", now.Add(-2*time.Hour), policy)
	if err := s.InsertInvitation(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	return id, inv
}

func seedSession(t *testing.T, d *db.DB, uid, hash string) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), `INSERT INTO sessions (user_id, token_hash, last_seen_at, expires_at)
		VALUES ($1, $2, now(), now() + interval '8 hours')`, uid, []byte(hash)); err != nil {
		t.Fatal(err)
	}
}

func applyResend(now time.Time) Mutator {
	return func(u base.User, inv *base.Invitation) error {
		if err := checkResendTarget(u, inv); err != nil {
			return err
		}
		if err := inv.RegisterResend(now, policy); err != nil {
			return err
		}
		inv.Reissue(now, policy)
		return nil
	}
}

func TestIssueRepo_I1_Success(t *testing.T) {
	d, r, s := setupRepo(t)
	uid, _ := seedInvitee(t, d, s, true)
	other, _ := seedInvitee(t, d, s, true)
	seedSession(t, d, uid, "a")
	seedSession(t, d, uid, "b")
	seedSession(t, d, other, "c")
	now := time.Now().UTC().Truncate(time.Microsecond)
	out, err := r.Issue(context.Background(), IssueInput{UserID: uid, Hash: "newhash", Now: now, Apply: applyResend(now)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Revoked != 2 || out.User.PasswordHash != "newhash" {
		t.Fatalf("%+v", out)
	}
	u, _, _ := s.FindUserByID(context.Background(), uid)
	inv, _, _ := s.FindInvitation(context.Background(), uid)
	if u.PasswordHash != "newhash" || !u.MustChangePassword || inv.ResendCount != 1 || inv.DeliveryStatus != base.DeliveryPending ||
		!inv.ExpiresAt.Equal(now.Add(policy.TTL)) || !inv.IssuedAt.Equal(now) || inv.ResendWindowCount != 1 {
		t.Fatalf("%+v %+v", u, inv)
	}
	var n int
	_ = d.GetContext(context.Background(), &n, `SELECT count(*) FROM sessions WHERE user_id=$1`, other)
	if n != 1 {
		t.Fatal("sesi user lain tersentuh")
	}
}

func TestIssueRepo_I2_RollbackOnApplyError(t *testing.T) {
	d, r, s := setupRepo(t)
	uid, _ := seedInvitee(t, d, s, false) // nonaktif -> 409
	seedSession(t, d, uid, "a")
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err := r.Issue(context.Background(), IssueInput{UserID: uid, Hash: "x", Now: now, Apply: applyResend(now)})
	if err == nil {
		t.Fatal("harus gagal")
	}
	u, _, _ := s.FindUserByID(context.Background(), uid)
	inv, _, _ := s.FindInvitation(context.Background(), uid)
	var n int
	_ = d.GetContext(context.Background(), &n, `SELECT count(*) FROM sessions WHERE user_id=$1`, uid)
	if u.PasswordHash != "old" || n != 1 || inv.ResendCount != 0 {
		t.Fatalf("%+v %+v %d", u, inv, n)
	}
}

func TestIssueRepo_I2_RateLimitedKeepsCounter(t *testing.T) {
	d, r, s := setupRepo(t)
	uid, _ := seedInvitee(t, d, s, true)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < policy.ResendMaxPerHour; i++ {
		if _, err := r.Issue(context.Background(), IssueInput{UserID: uid, Hash: "h", Now: now, Apply: applyResend(now)}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := r.Issue(context.Background(), IssueInput{UserID: uid, Hash: "zzz", Now: now, Apply: applyResend(now)})
	if err == nil {
		t.Fatal("harus 429")
	}
	inv, _, _ := s.FindInvitation(context.Background(), uid)
	u, _, _ := s.FindUserByID(context.Background(), uid)
	if inv.ResendWindowCount != policy.ResendMaxPerHour || u.PasswordHash == "zzz" {
		t.Fatalf("%+v", inv)
	}
}

func TestIssueRepo_I4_ConcurrentNoLostUpdate(t *testing.T) {
	d, r, s := setupRepo(t)
	uid, _ := seedInvitee(t, d, s, true)
	now := time.Now().UTC().Truncate(time.Microsecond)
	errc := make(chan error, 2)
	for _, h := range []string{"h1", "h2"} {
		go func(h string) {
			_, err := r.Issue(context.Background(), IssueInput{UserID: uid, Hash: h, Now: now, Apply: applyResend(now)})
			errc <- err
		}(h)
	}
	for i := 0; i < 2; i++ {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
	inv, _, _ := s.FindInvitation(context.Background(), uid)
	if inv.ResendWindowCount != 2 || inv.ResendCount != 2 {
		t.Fatalf("%+v", inv)
	}
}

func TestIssueRepo_I5_AfterAccept(t *testing.T) {
	d, r, s := setupRepo(t)
	uid, _ := seedInvitee(t, d, s, true)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if ok, err := s.AcceptInvitation(context.Background(), uid, now); err != nil || !ok {
		t.Fatal(err)
	}
	_, err := r.Issue(context.Background(), IssueInput{UserID: uid, Hash: "x", Now: now, Apply: applyResend(now)})
	if !errors.Is(err, base.ErrInvitationNotPending) {
		t.Fatalf("%v", err)
	}
}

func TestIssueRepo_I6_ExpiredReissued(t *testing.T) {
	d, r, s := setupRepo(t)
	uid, _ := seedInvitee(t, d, s, true)
	now := time.Now().UTC().Truncate(time.Microsecond).Add(10 * 24 * time.Hour) // jauh setelah kedaluwarsa
	out, err := r.Issue(context.Background(), IssueInput{UserID: uid, Hash: "x", Now: now, Apply: applyResend(now)})
	if err != nil || !out.Invitation.ExpiresAt.After(now) {
		t.Fatalf("%v %+v", err, out)
	}
}

func TestIssueRepo_I7_MarkDelivery(t *testing.T) {
	d, r, s := setupRepo(t)
	uid, _ := seedInvitee(t, d, s, true)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := r.MarkDelivery(ctx, uid, base.DeliverySent, now); err != nil {
		t.Fatal(err)
	}
	inv, _, _ := s.FindInvitation(ctx, uid)
	if inv.DeliveryStatus != "sent" || inv.SentAt == nil || !inv.SentAt.Equal(now) {
		t.Fatalf("%+v", inv)
	}
	if err := r.MarkDelivery(ctx, uid, base.DeliveryFailed, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	inv, _, _ = s.FindInvitation(ctx, uid)
	if inv.DeliveryStatus != "failed" || !inv.SentAt.Equal(now) {
		t.Fatalf("%+v", inv)
	}
	if ok, err := s.AcceptInvitation(ctx, uid, now); err != nil || !ok {
		t.Fatal(err)
	}
	if err := r.MarkDelivery(ctx, uid, base.DeliveryNotConfigured, now); err != nil {
		t.Fatal(err)
	}
	inv, _, _ = s.FindInvitation(ctx, uid)
	if inv.DeliveryStatus != "failed" {
		t.Fatalf("no-op setelah diterima: %+v", inv)
	}
}

func TestIssueRepo_I8_UserErrors(t *testing.T) {
	_, r, _ := setupRepo(t)
	now := time.Now().UTC()
	for _, id := range []string{"bukan-uuid", "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21"} {
		_, err := r.Issue(context.Background(), IssueInput{UserID: id, Hash: "x", Now: now, Apply: applyResend(now)})
		if !errors.Is(err, base.ErrUserNotFound) {
			t.Fatalf("%s: %v", id, err)
		}
	}
}
