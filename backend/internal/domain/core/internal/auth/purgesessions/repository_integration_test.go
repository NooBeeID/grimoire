//go:build integration

package purgesessions

import (
	"bytes"
	"context"
	"crypto/rand"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
	"grimoire/migrations"
)

var (
	N      = time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	idle   = 12 * time.Hour
	window = 15 * time.Minute
)

func setup(t *testing.T) (*db.DB, *Usecase, *bytes.Buffer) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	buf := &bytes.Buffer{}
	uc := New(NewRepository(d), func() time.Time { return N }, idle, window, slog.New(slog.NewTextHandler(buf, nil)))
	return d, uc, buf
}

func seedUser(t *testing.T, d *db.DB, email string) string {
	t.Helper()
	var id string
	err := d.Get(&id, `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
VALUES ($1,'U','h','developer',true,false) RETURNING id::text`, email)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedSession(t *testing.T, d *db.DB, userID string, lastSeen, expires time.Time) {
	t.Helper()
	h := make([]byte, 32)
	_, _ = rand.Read(h)
	_, err := d.Exec(`INSERT INTO sessions (user_id, token_hash, created_at, last_seen_at, expires_at) VALUES ($1,$2,$3,$4,$5)`,
		userID, h, N.Add(-24*time.Hour), lastSeen, expires)
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func seedThrottle(t *testing.T, d *db.DB, email string, started time.Time, locked *time.Time) {
	t.Helper()
	_, err := d.Exec(`INSERT INTO login_throttles (email, failed_count, window_started_at, locked_until) VALUES ($1,3,$2,$3)`, email, started, locked)
	if err != nil {
		t.Fatalf("seed throttle: %v", err)
	}
}

func count(t *testing.T, d *db.DB, table string) int {
	t.Helper()
	var n int
	if err := d.Get(&n, "SELECT count(*) FROM "+table); err != nil {
		t.Fatal(err)
	}
	return n
}

func tp(v time.Time) *time.Time { return &v }

func TestDeleteExpiredSessions_Cases(t *testing.T) {
	cases := []struct {
		name          string
		lastSeen, exp time.Time
		deleted       bool
	}{
		{"absolute expired", N, N.Add(-time.Second), true},
		{"absolute valid", N, N.Add(time.Second), false},
		{"idle expired", N.Add(-idle - time.Second), N.Add(48 * time.Hour), true},
		{"idle valid", N.Add(-11 * time.Hour), N.Add(48 * time.Hour), false},
		{"boundary strict", N.Add(-idle), N, false},
		{"both conditions", N.Add(-idle - time.Hour), N.Add(-time.Hour), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, uc, _ := setup(t)
			seedSession(t, d, seedUser(t, d, "a@x.id"), c.lastSeen, c.exp)
			if err := uc.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := count(t, d, "sessions") == 0; got != c.deleted {
				t.Fatalf("deleted = %v, want %v", got, c.deleted)
			}
		})
	}
}

func TestDeleteExpiredSessions_CountsAndKeepsValid(t *testing.T) {
	d, uc, buf := setup(t)
	u := seedUser(t, d, "a@x.id")
	for i := 0; i < 3; i++ {
		seedSession(t, d, u, N, N.Add(-time.Duration(i+1)*time.Minute))
		seedSession(t, d, u, N, N.Add(time.Hour))
	}
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, "sessions"); n != 3 {
		t.Fatalf("sisa %d", n)
	}
	if !strings.Contains(buf.String(), "deleted_sessions=3") {
		t.Fatal(buf.String())
	}
	if count(t, d, "users") != 1 {
		t.Fatal("users berubah")
	}
}

func TestDeleteExpiredSessions_AgreesWithIsValid(t *testing.T) {
	d, uc, _ := setup(t)
	u := seedUser(t, d, "a@x.id")
	offs := []time.Duration{-time.Second, 0, time.Second}
	type pair struct{ ls, ex time.Time }
	var pairs []pair
	for _, a := range offs {
		for _, b := range offs {
			p := pair{N.Add(-idle + a), N.Add(b)}
			pairs = append(pairs, p)
			seedSession(t, d, u, p.ls, p.ex)
		}
	}
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var remaining []time.Time
	if err := d.Select(&remaining, `SELECT expires_at FROM sessions`); err != nil {
		t.Fatal(err)
	}
	for _, p := range pairs {
		s := base.Session{LastSeenAt: p.ls, ExpiresAt: p.ex}
		if !s.IsValid(N, idle) {
			continue
		}
		found := false
		for _, e := range remaining {
			if e.Equal(p.ex) {
				found = true
			}
		}
		if !found {
			t.Fatalf("sesi valid terhapus: %+v", p)
		}
	}
}

func TestDeleteExpiredThrottles_MatchesExpiredOracle(t *testing.T) {
	policy := base.ThrottlePolicy{Window: window}
	starts := []time.Time{N.Add(-window - time.Second), N.Add(-window), N.Add(-5 * time.Minute)}
	locks := []*time.Time{nil, tp(N.Add(-time.Second)), tp(N), tp(N.Add(5 * time.Minute))}
	d, uc, _ := setup(t)
	type row struct {
		email string
		th    base.LoginThrottle
	}
	var rows []row
	i := 0
	for _, s := range starts {
		for _, l := range locks {
			i++
			email := "u" + string(rune('a'+i)) + "@x.id"
			seedThrottle(t, d, email, s, l)
			rows = append(rows, row{email, base.LoginThrottle{Email: email, WindowStartedAt: s, LockedUntil: l}})
		}
	}
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		var n int
		if err := d.Get(&n, `SELECT count(*) FROM login_throttles WHERE email=$1`, r.email); err != nil {
			t.Fatal(err)
		}
		if gone := n == 0; gone != r.th.Expired(N, policy) {
			t.Errorf("%+v: gone=%v expired=%v", r.th, gone, r.th.Expired(N, policy))
		}
	}
}

func TestDeleteExpiredThrottles_Explicit(t *testing.T) {
	d, uc, buf := setup(t)
	seedThrottle(t, d, "old@x.id", N.Add(-16*time.Minute), nil)
	seedThrottle(t, d, "locked@x.id", N.Add(-time.Hour), tp(N.Add(5*time.Minute)))
	seedThrottle(t, d, "Foo@X.id", N.Add(-time.Hour), tp(N.Add(-time.Second)))
	seedThrottle(t, d, "open@x.id", N.Add(-5*time.Minute), tp(N.Add(-time.Second)))
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, "login_throttles"); n != 2 {
		t.Fatalf("sisa %d", n)
	}
	if !strings.Contains(buf.String(), "deleted_throttles=2") {
		t.Fatal(buf.String())
	}
}

func TestRun_IdempotentAndEmpty(t *testing.T) {
	d, uc, buf := setup(t)
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	seedSession(t, d, seedUser(t, d, "a@x.id"), N.Add(-idle-time.Hour), N.Add(time.Hour))
	seedThrottle(t, d, "old@x.id", N.Add(-time.Hour), nil)
	buf.Reset()
	for i := 0; i < 2; i++ {
		if err := uc.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(buf.String(), "deleted_sessions=0 deleted_throttles=0") {
		t.Fatalf("run kedua harus 0/0: %s", buf.String())
	}
}

func TestRun_DBError(t *testing.T) {
	_, uc, _ := setup(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := uc.Run(ctx); err == nil {
		t.Fatal("harus error")
	}
}
