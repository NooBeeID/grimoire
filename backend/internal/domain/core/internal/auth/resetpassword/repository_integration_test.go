//go:build integration

package resetpassword

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

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

func seedUser(t *testing.T, d *db.DB, active bool) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, is_active)
		VALUES ('seed-'||gen_random_uuid()::text||'@x.id', 'Budi', 'old', 'developer', $1) RETURNING id::text`, active); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedSession(t *testing.T, d *db.DB, uid string, hash string) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), `INSERT INTO sessions (user_id, token_hash, last_seen_at, expires_at)
		VALUES ($1, $2, now(), now() + interval '8 hours')`, uid, []byte(hash)); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, d *db.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.GetContext(context.Background(), &n, q, args...); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestResetRepo_UpdatesHashFlagAndTimestamp(t *testing.T) {
	d, r := setupRepo(t)
	uid := seedUser(t, d, true)
	now := time.Now().UTC().Truncate(time.Microsecond)
	u, _, err := r.Reset(context.Background(), ResetInput{UserID: uid, Hash: "new", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != "new" || !u.MustChangePassword || !u.UpdatedAt.Equal(now) || u.Role != "developer" || !u.IsActive {
		t.Fatalf("%+v", u)
	}
}

func TestResetRepo_DeletesAllTargetSessions(t *testing.T) {
	d, r := setupRepo(t)
	uid, other := seedUser(t, d, true), seedUser(t, d, true)
	for _, h := range []string{"a", "b", "c"} {
		seedSession(t, d, uid, h)
	}
	seedSession(t, d, other, "z")
	_, n, err := r.Reset(context.Background(), ResetInput{UserID: uid, Hash: "new", Now: time.Now()})
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, uid) != 0 ||
		count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, other) != 1 {
		t.Fatal("sesi salah")
	}
}

func TestResetRepo_UserNotFound_NoSideEffects(t *testing.T) {
	d, r := setupRepo(t)
	uid := seedUser(t, d, true)
	seedSession(t, d, uid, "a")
	_, _, err := r.Reset(context.Background(), ResetInput{UserID: uuid.NewString(), Hash: "x", Now: time.Now()})
	if err != ErrTargetGone {
		t.Fatalf("err=%v", err)
	}
	if count(t, d, `SELECT count(*) FROM sessions`) != 1 {
		t.Fatal("sesi terhapus")
	}
}

func TestResetRepo_Atomic(t *testing.T) {
	d, r := setupRepo(t)
	uid := seedUser(t, d, true)
	seedSession(t, d, uid, "a")
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `CREATE FUNCTION fail_sess_del() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'boom'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `CREATE TRIGGER fail_sess BEFORE DELETE ON sessions FOR EACH ROW EXECUTE FUNCTION fail_sess_del()`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Reset(ctx, ResetInput{UserID: uid, Hash: "new", Now: time.Now()}); err == nil {
		t.Fatal("harus error")
	}
	if count(t, d, `SELECT count(*) FROM users WHERE id=$1 AND password_hash='old'`, uid) != 1 ||
		count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, uid) != 1 {
		t.Fatal("tidak rollback")
	}
}

func TestResetRepo_InactiveTarget(t *testing.T) {
	d, r := setupRepo(t)
	uid := seedUser(t, d, false)
	u, _, err := r.Reset(context.Background(), ResetInput{UserID: uid, Hash: "new", Now: time.Now()})
	if err != nil || u.IsActive || u.PasswordHash != "new" {
		t.Fatalf("u=%+v err=%v", u, err)
	}
}

func TestResetRepo_FindUserByID(t *testing.T) {
	d, r := setupRepo(t)
	uid := seedUser(t, d, true)
	if u, ok, err := r.FindUserByID(context.Background(), uid); err != nil || !ok || u.ID != uid {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, ok, err := r.FindUserByID(context.Background(), "abc"); err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
