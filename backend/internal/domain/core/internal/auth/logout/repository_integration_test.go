//go:build integration

package logout

import (
	"context"
	"testing"

	"grimoire/internal/infra/db"
	"grimoire/migrations"
)

func setupRepo(t *testing.T) (*db.DB, Repository) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, NewRepository(d)
}

func seedUser(t *testing.T, d *db.DB) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, is_active)
		VALUES ('seed-'||gen_random_uuid()::text||'@x.id', 'Budi', 'h', 'developer', true) RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedSession(t *testing.T, d *db.DB, uid string, hash []byte) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), `INSERT INTO sessions (user_id, token_hash, last_seen_at, expires_at)
		VALUES ($1, $2, now(), now() + interval '8 hours')`, uid, hash); err != nil {
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

func TestRepo_DeleteSessionByHash_DeletesOnlyThatSession(t *testing.T) {
	d, r := setupRepo(t)
	uid := seedUser(t, d)
	seedSession(t, d, uid, []byte("h1"))
	seedSession(t, d, uid, []byte("h2"))
	n, err := r.DeleteSessionByHash(context.Background(), []byte("h1"))
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if c := count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, uid); c != 1 {
		t.Fatalf("sisa=%d", c)
	}
}

func TestRepo_DeleteSessionByHash_UnknownHash(t *testing.T) {
	_, r := setupRepo(t)
	n, err := r.DeleteSessionByHash(context.Background(), []byte("nope"))
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestRepo_DeleteSessionByHash_DoesNotTouchUser(t *testing.T) {
	d, r := setupRepo(t)
	uid := seedUser(t, d)
	seedSession(t, d, uid, []byte("h1"))
	if _, err := r.DeleteSessionByHash(context.Background(), []byte("h1")); err != nil {
		t.Fatal(err)
	}
	if c := count(t, d, `SELECT count(*) FROM users WHERE id=$1`, uid); c != 1 {
		t.Fatal("user terhapus")
	}
}
