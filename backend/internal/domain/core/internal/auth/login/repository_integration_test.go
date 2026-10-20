//go:build integration

package login

import (
	"context"
	"sync"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
	"grimoire/migrations"
)

var policy = base.ThrottlePolicy{MaxFailures: 5, Window: 15 * time.Minute, LockDuration: 15 * time.Minute}

func setupRepo(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, NewRepository(d)
}

func seedUser(t *testing.T, d *db.DB, email string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
		VALUES ($1,'Budi','h','developer',true,false) RETURNING id::text`, email); err != nil {
		t.Fatal(err)
	}
	return id
}

func fail(now time.Time) func(*base.LoginThrottle) {
	return func(t *base.LoginThrottle) { t.RegisterFailure(now, policy) }
}

func TestRepo_GetThrottle_NoRow(t *testing.T) {
	_, r := setupRepo(t)
	th, err := r.GetThrottle(context.Background(), "x@tim.id")
	if err != nil || th.Email != "x@tim.id" || th.FailedCount != 0 || th.LockedUntil != nil || !th.WindowStartedAt.IsZero() {
		t.Fatalf("%+v %v", th, err)
	}
}

func TestRepo_UpdateThrottle_CreatesAndIncrements(t *testing.T) {
	_, r := setupRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 1; i <= 5; i++ {
		th, err := r.UpdateThrottle(ctx, "x@tim.id", now, fail(now))
		if err != nil || th.FailedCount != i {
			t.Fatalf("i=%d %+v %v", i, th, err)
		}
		if (i == 5) != (th.LockedUntil != nil) {
			t.Fatalf("i=%d lock=%v", i, th.LockedUntil)
		}
	}
	got, err := r.GetThrottle(ctx, "X@Tim.id") // citext
	if err != nil || got.FailedCount != 5 || got.LockedUntil == nil || !got.IsLocked(now) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestRepo_UpdateThrottle_ConcurrentFailures(t *testing.T) {
	_, r := setupRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	big := base.ThrottlePolicy{MaxFailures: 1000, Window: time.Hour, LockDuration: time.Hour}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.UpdateThrottle(ctx, "c@tim.id", now, func(t *base.LoginThrottle) { t.RegisterFailure(now, big) }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	th, _ := r.GetThrottle(ctx, "c@tim.id")
	if th.FailedCount != 10 {
		t.Fatalf("update hilang: %d", th.FailedCount)
	}
}

func TestRepo_UpdateThrottle_WindowReset(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-time.Hour)
	if _, err := d.Exec(`INSERT INTO login_throttles (email, failed_count, window_started_at) VALUES ('w@tim.id', 4, $1)`, old); err != nil {
		t.Fatal(err)
	}
	th, err := r.UpdateThrottle(ctx, "w@tim.id", now, fail(now))
	if err != nil || th.FailedCount != 1 || th.LockedUntil != nil {
		t.Fatalf("%+v %v", th, err)
	}
}

func newSess(userID string, hash byte, ip, ua string, now time.Time) base.Session {
	return base.NewSession(userID, []byte{hash, hash, hash}, now, base.SessionConfig{AbsoluteTimeout: time.Hour},
		base.RequestMeta{IP: ip, UserAgent: ua})
}

func TestRepo_CompleteLogin_Atomic(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	uid := seedUser(t, d, "a@tim.id")
	now := time.Now().UTC()
	if _, err := r.UpdateThrottle(ctx, "a@tim.id", now, fail(now)); err != nil {
		t.Fatal(err)
	}
	var updBefore time.Time
	_ = d.GetContext(ctx, &updBefore, `SELECT updated_at FROM users WHERE id=$1`, uid)
	if err := r.CompleteLogin(ctx, "a@tim.id", uid, newSess(uid, 7, "10.0.0.5", "UA", now), now); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.GetContext(ctx, &n, `SELECT count(*) FROM login_throttles WHERE email='a@tim.id'`)
	if n != 0 {
		t.Fatal("throttle harus terhapus")
	}
	var hash []byte
	var ip, ua string
	if err := d.QueryRowxContext(ctx, `SELECT token_hash, host(ip), user_agent FROM sessions WHERE user_id=$1`, uid).Scan(&hash, &ip, &ua); err != nil {
		t.Fatal(err)
	}
	if string(hash) != string([]byte{7, 7, 7}) || ip != "10.0.0.5" || ua != "UA" {
		t.Fatalf("%v %s %s", hash, ip, ua)
	}
	var last *time.Time
	var updAfter time.Time
	_ = d.QueryRowxContext(ctx, `SELECT last_login_at, updated_at FROM users WHERE id=$1`, uid).Scan(&last, &updAfter)
	if last == nil || !updAfter.Equal(updBefore) {
		t.Fatalf("last=%v upd %v -> %v", last, updBefore, updAfter)
	}
}

func TestRepo_CompleteLogin_RollbackOnFailure(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	uid := seedUser(t, d, "b@tim.id")
	now := time.Now().UTC()
	if _, err := r.UpdateThrottle(ctx, "b@tim.id", now, fail(now)); err != nil {
		t.Fatal(err)
	}
	bad := "00000000-0000-0000-0000-000000000001"
	if err := r.CompleteLogin(ctx, "b@tim.id", bad, newSess(bad, 1, "", "", now), now); err == nil {
		t.Fatal("harus gagal (FK)")
	}
	var n int
	_ = d.GetContext(ctx, &n, `SELECT count(*) FROM login_throttles WHERE email='b@tim.id'`)
	var last *time.Time
	_ = d.GetContext(ctx, &last, `SELECT last_login_at FROM users WHERE id=$1`, uid)
	if n != 1 || last != nil {
		t.Fatalf("rollback gagal n=%d last=%v", n, last)
	}
}

func TestRepo_CompleteLogin_IPHandling(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	uid := seedUser(t, d, "i@tim.id")
	now := time.Now().UTC()
	cases := []struct {
		ip   string
		want *string
	}{{"", nil}, {"bukan-ip", nil}, {"192.168.1.9", ptr("192.168.1.9")}, {"2001:db8::1", ptr("2001:db8::1")}}
	for i, c := range cases {
		s := newSess(uid, byte(10+i), c.ip, "", now)
		if err := r.CompleteLogin(ctx, "i@tim.id", uid, s, now); err != nil {
			t.Fatalf("%q: %v", c.ip, err)
		}
		var got, ua *string
		if err := d.QueryRowxContext(ctx, `SELECT host(ip), user_agent FROM sessions WHERE token_hash=$1`, s.TokenHash).Scan(&got, &ua); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) || ua != nil {
			t.Fatalf("%q -> %v ua=%v", c.ip, got, ua)
		}
	}
}

func ptr(s string) *string { return &s }

func TestRepo_FindUserByEmail_CaseInsensitive(t *testing.T) {
	d, r := setupRepo(t)
	seedUser(t, d, "Budi@Tim.ID")
	u, found, err := r.FindUserByEmail(context.Background(), "budi@tim.id")
	if err != nil || !found || u.Name != "Budi" {
		t.Fatalf("%+v %v %v", u, found, err)
	}
}
