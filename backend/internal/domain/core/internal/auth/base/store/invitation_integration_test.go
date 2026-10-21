//go:build integration

package store

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
)

var invPolicy = base.InvitationPolicy{TTL: 168 * time.Hour, MaxPerInviterPerHour: 20, ResendMaxPerHour: 3, PurgeAfter: 720 * time.Hour}

func seedInvitee(t *testing.T, s *Store, email, inviter string) base.Invitation {
	t.Helper()
	u, err := s.InsertUser(context.Background(), newUser(email, base.RoleDeveloper))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	return base.NewInvitation(u.ID, inviter, "Owner", now, invPolicy)
}

func TestInvitationStore_RoundTripAllColumns(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	adm, _ := s.InsertUser(ctx, newUser("adm@x.id", base.RoleSuperAdmin))
	inv := seedInvitee(t, s, "a@x.id", adm.ID)
	sent := inv.CreatedAt.Add(time.Second)
	win := inv.CreatedAt.Add(2 * time.Second)
	inv.SentAt, inv.ResendWindowStartedAt = &sent, &win
	inv.DeliveryStatus, inv.ResendCount, inv.ResendWindowCount = base.DeliverySent, 2, 1
	if err := s.InsertInvitation(ctx, inv); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.FindInvitation(ctx, inv.UserID)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if got.UserID != inv.UserID || got.InvitedBy != adm.ID || got.InvitedByLabel == nil || *got.InvitedByLabel != "Owner" ||
		!got.IssuedAt.Equal(inv.IssuedAt) || !got.ExpiresAt.Equal(inv.ExpiresAt) || !got.CreatedAt.Equal(inv.CreatedAt) ||
		got.SentAt == nil || !got.SentAt.Equal(sent) || got.AcceptedAt != nil ||
		got.ResendWindowStartedAt == nil || !got.ResendWindowStartedAt.Equal(win) ||
		got.DeliveryStatus != base.DeliverySent || got.ResendCount != 2 || got.ResendWindowCount != 1 {
		t.Fatalf("%+v", got)
	}
	if _, ok, err := s.FindInvitation(ctx, "bukan-uuid"); ok || err != nil {
		t.Fatalf("id bukan uuid -> not found: %v %v", ok, err)
	}
	if _, ok, _ := s.FindInvitation(ctx, adm.ID); ok {
		t.Fatal("user tanpa undangan -> not found")
	}
}

func TestInvitationStore_UpdateAndAccept(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	adm, _ := s.InsertUser(ctx, newUser("adm@x.id", base.RoleSuperAdmin))
	inv := seedInvitee(t, s, "a@x.id", adm.ID)
	if err := s.InsertInvitation(ctx, inv); err != nil {
		t.Fatal(err)
	}
	later := inv.CreatedAt.Add(time.Hour)
	inv.Reissue(later, invPolicy)
	inv.MarkDelivery(base.DeliverySent, later)
	if err := inv.RegisterResend(later, invPolicy); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInvitation(ctx, inv); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.FindInvitation(ctx, inv.UserID)
	if !got.ExpiresAt.Equal(inv.ExpiresAt) || got.DeliveryStatus != base.DeliverySent || got.ResendCount != 1 || got.ResendWindowCount != 1 || got.SentAt == nil {
		t.Fatalf("%+v", got)
	}
	if err := s.UpdateInvitation(ctx, base.Invitation{UserID: "00000000-0000-0000-0000-000000000000"}); !errors.Is(err, base.ErrInvitationNotPending) {
		t.Fatalf("baris tidak ada: %v", err)
	}

	ok, err := s.AcceptInvitation(ctx, inv.UserID, later)
	if err != nil || !ok {
		t.Fatalf("pertama true: %v %v", ok, err)
	}
	if ok, _ := s.AcceptInvitation(ctx, inv.UserID, later); ok {
		t.Fatal("kedua false")
	}
	if ok, _ := s.AcceptInvitation(ctx, "bukan-uuid", later); ok {
		t.Fatal("bukan uuid false")
	}
	got, _, _ = s.FindInvitation(ctx, inv.UserID)
	if got.AcceptedAt == nil {
		t.Fatal("AcceptedAt harus terisi")
	}
}

func TestInvitationStore_CountCreatedBy(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	a, _ := s.InsertUser(ctx, newUser("a@x.id", base.RoleSuperAdmin))
	b, _ := s.InsertUser(ctx, newUser("b@x.id", base.RoleSuperAdmin))
	now := time.Now().UTC().Truncate(time.Microsecond)
	mk := func(email, inviter string, at time.Time) {
		u, err := s.InsertUser(ctx, newUser(email, base.RoleDeveloper))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.InsertInvitation(ctx, base.NewInvitation(u.ID, inviter, "", at, invPolicy)); err != nil {
			t.Fatal(err)
		}
	}
	mk("i1@x.id", a.ID, now.Add(-30*time.Minute))
	mk("i2@x.id", a.ID, now.Add(-10*time.Minute))
	mk("i3@x.id", a.ID, now.Add(-3*time.Hour)) // di luar jendela
	mk("i4@x.id", b.ID, now.Add(-5*time.Minute))
	n, oldest, err := s.CountInvitationsCreatedBy(ctx, a.ID, now.Add(-time.Hour))
	if err != nil || n != 2 || !oldest.Equal(now.Add(-30*time.Minute)) {
		t.Fatalf("%d %v %v", n, oldest, err)
	}
	n, oldest, _ = s.CountInvitationsCreatedBy(ctx, a.ID, now.Add(time.Hour))
	if n != 0 || !oldest.IsZero() {
		t.Fatalf("kosong: %d %v", n, oldest)
	}
	if n, _, _ := s.CountInvitationsCreatedBy(ctx, "bukan-uuid", now); n != 0 {
		t.Fatal("bukan uuid -> 0")
	}
}

func TestInvitationStore_LockInvitationBlocksSecondTx(t *testing.T) {
	d, s := setup(t)
	ctx := context.Background()
	adm, _ := s.InsertUser(ctx, newUser("adm@x.id", base.RoleSuperAdmin))
	inv := seedInvitee(t, s, "a@x.id", adm.ID)
	if err := s.InsertInvitation(ctx, inv); err != nil {
		t.Fatal(err)
	}
	var second atomic.Int64
	first := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		done <- d.BeginTx(ctx, func(tx *sqlx.Tx) error {
			if _, ok, err := s.WithTx(tx).LockInvitation(ctx, inv.UserID); err != nil || !ok {
				return errors.Join(err, errors.New("lock pertama gagal"))
			}
			close(first)
			<-release
			return nil
		})
	}()
	<-first
	go func() {
		done <- d.BeginTx(ctx, func(tx *sqlx.Tx) error {
			_, _, err := s.WithTx(tx).LockInvitation(ctx, inv.UserID)
			second.Store(time.Now().UnixNano())
			return err
		})
	}()
	time.Sleep(300 * time.Millisecond)
	if second.Load() != 0 {
		t.Fatal("tx kedua seharusnya tertahan")
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if second.Load() == 0 {
		t.Fatal("tx kedua harus lanjut setelah rilis")
	}
}

func TestInvitationMigration_ConstraintsAndCascade(t *testing.T) {
	d, s := setup(t)
	ctx := context.Background()
	adm, _ := s.InsertUser(ctx, newUser("adm@x.id", base.RoleSuperAdmin))
	inv := seedInvitee(t, s, "a@x.id", adm.ID)
	inv.DeliveryStatus = "bogus"
	if err := s.InsertInvitation(ctx, inv); err == nil {
		t.Fatal("CHECK delivery_status harus menolak nilai lain")
	}
	inv.DeliveryStatus = base.DeliveryPending
	if err := s.InsertInvitation(ctx, inv); err != nil {
		t.Fatal(err)
	}
	var idx []string
	if err := d.SelectContext(ctx, &idx, `SELECT indexname FROM pg_indexes WHERE tablename = 'user_invitations' ORDER BY indexname`); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"user_invitations_pending_expires_idx": false, "user_invitations_inviter_created_idx": false}
	for _, n := range idx {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, ok := range want {
		if !ok {
			t.Errorf("index %s tidak ada (%v)", n, idx)
		}
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM users WHERE id = $1::uuid`, inv.UserID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.FindInvitation(ctx, inv.UserID); ok {
		t.Fatal("hapus user harus meng-cascade undangan")
	}
}
