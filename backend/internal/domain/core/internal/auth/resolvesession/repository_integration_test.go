//go:build integration

package resolvesession

import (
	"context"
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

func seed(t *testing.T, d *db.DB, hash []byte, lastSeen time.Time) (userID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	if err := d.GetContext(ctx, &userID, `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
		VALUES ('seed-'||gen_random_uuid()::text||'@x.id', 'Budi', 'secret-hash', 'developer', true, true) RETURNING id::text`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := d.GetContext(ctx, &sessionID, `INSERT INTO sessions (user_id, token_hash, last_seen_at, expires_at)
		VALUES ($1, $2, $3::timestamptz, $3::timestamptz + interval '8 hours') RETURNING id::text`, userID, hash, lastSeen); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return
}

func TestResolveSessionRepo_FindJoinsUser(t *testing.T) {
	d, r := setupRepo(t)
	uid, sid := seed(t, d, []byte("hash-1"), time.Now().UTC())
	s, u, found, err := r.FindSession(context.Background(), []byte("hash-1"))
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if s.ID != sid || s.UserID != uid || u.ID != uid || u.Name != "Budi" || u.Role != "developer" ||
		!u.IsActive || !u.MustChangePassword || u.PasswordHash != "" {
		t.Fatalf("%+v %+v", s, u)
	}
}

func TestResolveSessionRepo_UnknownHash(t *testing.T) {
	_, r := setupRepo(t)
	_, _, found, err := r.FindSession(context.Background(), []byte("tidak-ada"))
	if found || err != nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func TestResolveSessionRepo_TouchConditional(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	base0 := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	_, sid := seed(t, d, []byte("hash-2"), base0)
	now := base0.Add(2 * time.Minute)
	changed, err := r.TouchSession(ctx, sid, now)
	if err != nil || !changed {
		t.Fatalf("first touch: %v %v", changed, err)
	}
	changed, err = r.TouchSession(ctx, sid, now.Add(30*time.Second))
	if err != nil || changed {
		t.Fatalf("second touch: %v %v", changed, err)
	}
	s, _, _, _ := r.FindSession(ctx, []byte("hash-2"))
	if !s.LastSeenAt.Equal(now) {
		t.Fatalf("last_seen %v want %v", s.LastSeenAt, now)
	}
}

func TestResolveSessionRepo_TouchConcurrent(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	base0 := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	_, sid := seed(t, d, []byte("hash-3"), base0)
	now := base0.Add(5 * time.Minute)
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.TouchSession(ctx, sid, now)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	s, _, _, _ := r.FindSession(ctx, []byte("hash-3"))
	if !s.LastSeenAt.Equal(now) {
		t.Fatalf("last_seen %v want %v", s.LastSeenAt, now)
	}
}

func TestResolveSessionRepo_CascadeOnUserDelete(t *testing.T) {
	d, r := setupRepo(t)
	uid, _ := seed(t, d, []byte("hash-4"), time.Now().UTC())
	if _, err := d.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, uid); err != nil {
		t.Fatal(err)
	}
	_, _, found, err := r.FindSession(context.Background(), []byte("hash-4"))
	if found || err != nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
}
