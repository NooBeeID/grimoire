//go:build integration

package inviteuser

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
	"grimoire/migrations"
)

func setupRepo(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, NewRepo(d)
}

func seedInviter(t *testing.T, d *db.DB, email string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
		VALUES ($1, 'Admin', 'h', 'super_admin', true, false) RETURNING id::text`, email); err != nil {
		t.Fatal(err)
	}
	return id
}

func realUsecase(r *Repo, st *fakeSend) (*Usecase, *fakeAudit) {
	a := &fakeAudit{}
	h := base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}, Rand: zeroRand{}}
	uc := NewUsecase(r, h, a, func() time.Time { return time.Now().UTC() }, zeroRandSeq{}, testPolicy, "https://g.id/login", st.fn)
	return uc, a
}

// zeroRandSeq memberi byte acak palsu yang aman dipakai paralel. Isinya berbeda tiap
// pemanggilan (counter atomik) agar password yang dibangkitkan tidak sama dua kali.
type zeroRandSeq struct{}

var randSeqCounter atomic.Uint64

func (zeroRandSeq) Read(p []byte) (int, error) {
	n := byte(randSeqCounter.Add(1))
	for i := range p {
		p[i] = byte(i*13+5) + n*31
	}
	return len(p), nil
}

func cnt(t *testing.T, d *db.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.GetContext(context.Background(), &n, q, args...); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepo_CreateInTx_PersistsUserAndInvitation(t *testing.T) {
	d, r := setupRepo(t)
	inviter := seedInviter(t, d, "adm@x.id")
	uc, _ := realUsecase(r, &fakeSend{})
	res, err := uc.Execute(context.Background(), Input{Email: "Baru@X.id", InvitedBy: inviter, InvitedByLabel: "Admin", Deliver: true, Via: ViaUsers})
	if err != nil || !res.Created {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	tgt, err := r.FindTarget(context.Background(), "baru@x.id")
	if err != nil || tgt.User == nil || tgt.Invitation == nil {
		t.Fatalf("tgt=%+v err=%v", tgt, err)
	}
	if tgt.User.Role != "developer" || !tgt.User.MustChangePassword || !strings.HasPrefix(tgt.User.PasswordHash, "$argon2id$") ||
		tgt.Invitation.DeliveryStatus != base.DeliverySent || tgt.Invitation.InvitedBy != inviter {
		t.Fatalf("tgt: %+v %+v", tgt.User, tgt.Invitation)
	}
}

func TestRepo_TxRollbackOnInvitationFailure(t *testing.T) {
	d, r := setupRepo(t)
	err := r.InTx(context.Background(), func(tx Tx) error {
		u, err := tx.InsertUser(context.Background(), base.User{Email: "y@x.id", Name: "Y", PasswordHash: "h", Role: base.RoleDeveloper, IsActive: true})
		if err != nil {
			return err
		}
		return tx.InsertInvitation(context.Background(), base.Invitation{UserID: u.ID, InvitedBy: "bukan-uuid", DeliveryStatus: base.DeliveryPending})
	})
	if err == nil {
		t.Fatal("harus gagal")
	}
	if n := cnt(t, d, `SELECT count(*) FROM users WHERE email='y@x.id'`); n != 0 {
		t.Fatalf("akun yatim: %d", n)
	}
}

func TestRepo_DuplicateEmailCaseInsensitive(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	mk := func(e string) error {
		return r.InTx(ctx, func(tx Tx) error {
			_, err := tx.InsertUser(ctx, base.User{Email: e, Name: "B", PasswordHash: "h", Role: base.RoleDeveloper, IsActive: true})
			return err
		})
	}
	if err := mk("Baru@X.id"); err != nil {
		t.Fatal(err)
	}
	if err := mk("baru@x.id"); !errors.Is(err, base.ErrEmailTaken) {
		t.Fatalf("err=%v", err)
	}
	if n := cnt(t, d, `SELECT count(*) FROM users WHERE lower(email)='baru@x.id'`); n != 1 {
		t.Fatalf("n=%d", n)
	}
}

func TestRepo_ConcurrentSameEmail(t *testing.T) {
	d, r := setupRepo(t)
	inviter := seedInviter(t, d, "adm@x.id")
	uc, _ := realUsecase(r, &fakeSend{})
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		created int
		other   []error
	)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := uc.Execute(context.Background(), Input{Email: "sama@x.id", InvitedBy: inviter, Deliver: true, Via: ViaUsers})
			mu.Lock()
			defer mu.Unlock()
			if err == nil && res.Created {
				created++
			} else if err != nil {
				var ee *errs.Error
				if !errors.As(err, &ee) || ee.Code != errs.CodeRateLimited {
					other = append(other, err)
				}
			}
		}()
	}
	wg.Wait()
	if created != 1 || len(other) != 0 {
		t.Fatalf("created=%d other=%v", created, other)
	}
	if cnt(t, d, `SELECT count(*) FROM users WHERE email='sama@x.id'`) != 1 || cnt(t, d, `SELECT count(*) FROM user_invitations`) != 1 {
		t.Fatal("baris ganda")
	}
}

func TestRepo_CountInvitedSince(t *testing.T) {
	d, r := setupRepo(t)
	a, b := seedInviter(t, d, "a@x.id"), seedInviter(t, d, "b@x.id")
	uc, _ := realUsecase(r, &fakeSend{})
	ctx := context.Background()
	for _, e := range []string{"1@x.id", "2@x.id"} {
		if _, err := uc.Execute(ctx, Input{Email: e, InvitedBy: a, Deliver: false, Via: ViaUsers}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := uc.Execute(ctx, Input{Email: "3@x.id", InvitedBy: b, Deliver: false, Via: ViaUsers}); err != nil {
		t.Fatal(err)
	}
	// reissue tidak menambah hitungan
	if _, err := uc.Execute(ctx, Input{Email: "1@x.id", InvitedBy: a, Deliver: true, Via: ViaUsers}); err != nil {
		t.Fatal(err)
	}
	n, oldest, err := r.CountInvitedSince(ctx, a, time.Now().Add(-time.Hour))
	if err != nil || n != 2 || oldest.IsZero() {
		t.Fatalf("n=%d oldest=%v err=%v", n, oldest, err)
	}
	if n, _, _ := r.CountInvitedSince(ctx, a, time.Now().Add(time.Hour)); n != 0 {
		t.Fatalf("n=%d", n)
	}
}

func TestRepo_ReissueTx_RevokesSessionsAndResetsHash(t *testing.T) {
	d, r := setupRepo(t)
	inviter := seedInviter(t, d, "adm@x.id")
	uc, _ := realUsecase(r, &fakeSend{status: base.DeliveryFailed})
	ctx := context.Background()
	first, err := uc.Execute(ctx, Input{Email: "w@x.id", InvitedBy: inviter, Deliver: false, Via: ViaUsers})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"s1", "s2"} {
		if _, err := d.ExecContext(ctx, `INSERT INTO sessions (user_id, token_hash, last_seen_at, expires_at) VALUES ($1,$2,now(), now()+interval '8 hours')`, first.User.ID, []byte(h)); err != nil {
			t.Fatal(err)
		}
	}
	oldHash := first.User.PasswordHash
	res, err := uc.Execute(ctx, Input{Email: "w@x.id", InvitedBy: inviter, Deliver: true, Via: ViaUsers})
	if err != nil || res.Created {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if n := cnt(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, first.User.ID); n != 0 {
		t.Fatalf("sesi=%d", n)
	}
	tgt, _ := r.FindTarget(ctx, "w@x.id")
	if tgt.User.PasswordHash == oldHash || !tgt.User.MustChangePassword || tgt.Invitation.ResendCount != 1 || tgt.Invitation.DeliveryStatus != base.DeliveryFailed {
		t.Fatalf("tgt: %+v %+v", tgt.User, tgt.Invitation)
	}
	if !tgt.Invitation.ExpiresAt.After(first.Invitation.ExpiresAt) {
		t.Fatal("expires_at harus maju")
	}
}

func TestRepo_LockOrder_NoDeadlock(t *testing.T) {
	d, r := setupRepo(t)
	inviter := seedInviter(t, d, "adm@x.id")
	uc, _ := realUsecase(r, &fakeSend{})
	ctx := context.Background()
	if _, err := uc.Execute(ctx, Input{Email: "l@x.id", InvitedBy: inviter, Deliver: false, Via: ViaUsers}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errc := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := uc.Execute(ctx, Input{Email: "l@x.id", InvitedBy: inviter, Deliver: true, Via: ViaUsers})
			errc <- err
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("deadlock")
	}
	close(errc)
	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepo_DeliveryRecordGuardedByIssuedAt(t *testing.T) {
	d, r := setupRepo(t)
	inviter := seedInviter(t, d, "adm@x.id")
	ctx := context.Background()
	var sendUC *Usecase
	st := &fakeSend{}
	uc, _ := realUsecase(r, st)
	sendUC = uc
	// saat kirim, request lain menerbitkan ulang (issued_at berubah di DB)
	sendUC.send = func(c context.Context, to string, _ base.EmailContent) string {
		if _, err := d.ExecContext(c, `UPDATE user_invitations SET issued_at = issued_at + interval '1 minute'`); err != nil {
			t.Error(err)
		}
		return base.DeliverySent
	}
	if _, err := uc.Execute(ctx, Input{Email: "g@x.id", InvitedBy: inviter, Deliver: true, Via: ViaUsers}); err != nil {
		t.Fatal(err)
	}
	tgt, _ := r.FindTarget(ctx, "g@x.id")
	if tgt.Invitation.DeliveryStatus != base.DeliveryPending {
		t.Fatalf("status tertimpa: %s", tgt.Invitation.DeliveryStatus)
	}
}
