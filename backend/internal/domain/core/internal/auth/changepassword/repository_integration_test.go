//go:build integration

package changepassword

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"grimoire/internal/infra/db"
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

func seedUser(t *testing.T, d *db.DB, email string, active bool) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
		VALUES ($1,'Budi','old-hash','developer',$2,true) RETURNING id::text`, email, active); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedSession(t *testing.T, d *db.DB, userID, tok string) []byte {
	t.Helper()
	h := []byte(tok)
	if _, err := d.ExecContext(context.Background(), `INSERT INTO sessions (user_id, token_hash, expires_at, last_seen_at)
		VALUES ($1::uuid,$2,now()+interval '1 day',now())`, userID, h); err != nil {
		t.Fatal(err)
	}
	return h
}

func count(t *testing.T, d *db.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.GetContext(context.Background(), &n, q, args...); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestChangeRepo_UpdatesHashAndFlag(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	keep := seedSession(t, d, id, "s1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, _, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "new-hash", KeepTokenHash: keep, Now: now}); err != nil {
		t.Fatal(err)
	}
	var row struct {
		Hash string    `db:"password_hash"`
		Must bool      `db:"must_change_password"`
		At   time.Time `db:"updated_at"`
	}
	if err := d.GetContext(context.Background(), &row, `SELECT password_hash, must_change_password, updated_at FROM users WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if row.Hash != "new-hash" || row.Must || !row.At.Equal(now) {
		t.Fatalf("%+v", row)
	}
}

func TestChangeRepo_RevokesOtherSessionsKeepsCurrent(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	other := seedUser(t, d, "b@tim.id", true)
	keep := seedSession(t, d, id, "s1")
	seedSession(t, d, id, "s2")
	seedSession(t, d, id, "s3")
	seedSession(t, d, other, "o1")
	n, _, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: keep, Now: time.Now()})
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if c := count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1::uuid`, id); c != 1 {
		t.Fatalf("sisa=%d", c)
	}
	if c := count(t, d, `SELECT count(*) FROM sessions WHERE token_hash=$1`, keep); c != 1 {
		t.Fatal("sesi saat ini hilang")
	}
	if c := count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1::uuid`, other); c != 1 {
		t.Fatal("sesi user lain tersentuh")
	}
}

func TestChangeRepo_StaleHashRollsBack(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	keep := seedSession(t, d, id, "s1")
	seedSession(t, d, id, "s2")
	_, _, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "salah", NewHash: "n", KeepTokenHash: keep, Now: time.Now()})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("%v", err)
	}
	if c := count(t, d, `SELECT count(*) FROM users WHERE id=$1::uuid AND password_hash='old-hash' AND must_change_password`, id); c != 1 {
		t.Fatal("user berubah")
	}
	if c := count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1::uuid`, id); c != 2 {
		t.Fatalf("sesi=%d", c)
	}
}

func TestChangeRepo_InactiveUserNotUpdated(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", false)
	_, _, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: []byte("k"), Now: time.Now()})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("%v", err)
	}
}

func TestChangeRepo_ConcurrentSameOldHash(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	keep := seedSession(t, d, id, "s1")
	var wg sync.WaitGroup
	errsC := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: keep, Now: time.Now()})
			errsC <- err
		}()
	}
	wg.Wait()
	close(errsC)
	ok, stale := 0, 0
	for err := range errsC {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrStale):
			stale++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || stale != 1 {
		t.Fatalf("ok=%d stale=%d", ok, stale)
	}
}

func seedInv(t *testing.T, d *db.DB, userID string, accepted *time.Time) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), `INSERT INTO user_invitations (user_id, invited_by, issued_at, expires_at, accepted_at)
		VALUES ($1::uuid, $1::uuid, now(), now()+interval '7 days', $2)`, userID, accepted); err != nil {
		t.Fatal(err)
	}
}

func acceptedAt(t *testing.T, d *db.DB, userID string) *time.Time {
	t.Helper()
	var at *time.Time
	if err := d.GetContext(context.Background(), &at, `SELECT accepted_at FROM user_invitations WHERE user_id=$1::uuid`, userID); err != nil {
		t.Fatal(err)
	}
	return at
}

func TestChangeRepo_Accept_MarksAcceptedAt(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	keep := seedSession(t, d, id, "s1")
	seedSession(t, d, id, "s2")
	seedInv(t, d, id, nil)
	now := time.Now().UTC().Truncate(time.Microsecond)
	revoked, accepted, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "new-hash", KeepTokenHash: keep, Now: now})
	if err != nil || !accepted || revoked != 1 {
		t.Fatalf("%d %v %v", revoked, accepted, err)
	}
	if at := acceptedAt(t, d, id); at == nil || !at.Equal(now) {
		t.Fatalf("%v", at)
	}
	if n := count(t, d, `SELECT count(*) FROM users WHERE id=$1::uuid AND password_hash='new-hash' AND NOT must_change_password`, id); n != 1 {
		t.Fatal("password tidak berubah")
	}
}

func TestChangeRepo_Accept_OnlyOnce(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	keep := seedSession(t, d, id, "s1")
	seedInv(t, d, id, nil)
	first := time.Now().UTC().Truncate(time.Microsecond)
	if _, acc, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "h2", KeepTokenHash: keep, Now: first}); err != nil || !acc {
		t.Fatalf("%v %v", acc, err)
	}
	_, acc, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "h2", NewHash: "h3", KeepTokenHash: keep, Now: first.Add(time.Hour)})
	if err != nil || acc {
		t.Fatalf("%v %v", acc, err)
	}
	if at := acceptedAt(t, d, id); at == nil || !at.Equal(first) {
		t.Fatalf("%v", at)
	}
}

func TestChangeRepo_NoInvitationRow(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	_, acc, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: []byte("k"), Now: time.Now()})
	if err != nil || acc || count(t, d, `SELECT count(*) FROM user_invitations`) != 0 {
		t.Fatalf("%v %v", acc, err)
	}
}

func TestChangeRepo_Stale_RollsBackAccept(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	seedInv(t, d, id, nil)
	_, acc, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "salah", NewHash: "n", KeepTokenHash: []byte("k"), Now: time.Now()})
	if !errors.Is(err, ErrStale) || acc || acceptedAt(t, d, id) != nil {
		t.Fatalf("%v %v", acc, err)
	}
	if count(t, d, `SELECT count(*) FROM users WHERE id=$1::uuid AND password_hash='old-hash'`, id) != 1 {
		t.Fatal("hash berubah")
	}
}

func TestChangeRepo_AlreadyAccepted(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	old := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	seedInv(t, d, id, &old)
	_, acc, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: []byte("k"), Now: time.Now()})
	if err != nil || acc {
		t.Fatalf("%v %v", acc, err)
	}
	if at := acceptedAt(t, d, id); at == nil || !at.Equal(old) {
		t.Fatalf("%v", at)
	}
}

func TestChangeRepo_ConcurrentChange_AcceptedExactlyOnce(t *testing.T) {
	d, r := setupRepo(t)
	id := seedUser(t, d, "a@tim.id", true)
	keep := seedSession(t, d, id, "s1")
	seedInv(t, d, id, nil)
	var wg sync.WaitGroup
	type res struct {
		acc bool
		err error
	}
	out := make(chan res, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, acc, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: keep, Now: time.Now()})
			out <- res{acc, err}
		}()
	}
	wg.Wait()
	close(out)
	acc, stale := 0, 0
	for o := range out {
		switch {
		case o.err == nil && o.acc:
			acc++
		case errors.Is(o.err, ErrStale):
			stale++
		default:
			t.Fatalf("%+v", o)
		}
	}
	if acc != 1 || stale != 1 || acceptedAt(t, d, id) == nil {
		t.Fatalf("acc=%d stale=%d", acc, stale)
	}
}
