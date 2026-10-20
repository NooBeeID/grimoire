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
	if _, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "new-hash", KeepTokenHash: keep, Now: now}); err != nil {
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
	n, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: keep, Now: time.Now()})
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
	_, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "salah", NewHash: "n", KeepTokenHash: keep, Now: time.Now()})
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
	_, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: []byte("k"), Now: time.Now()})
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
			_, err := r.Change(context.Background(), ChangeInput{UserID: id, OldHash: "old-hash", NewHash: "n", KeepTokenHash: keep, Now: time.Now()})
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
