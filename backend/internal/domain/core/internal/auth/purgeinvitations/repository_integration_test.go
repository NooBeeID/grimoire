//go:build integration

package purgeinvitations

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
	"grimoire/migrations"
)

var (
	N      = time.Date(2026, 10, 20, 3, 30, 0, 0, time.UTC)
	purge  = 720 * time.Hour
	cutoff = N.Add(-purge)
)

func setup(t *testing.T) (*db.DB, *Repository) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, NewRepository(d)
}

func seedUser(t *testing.T, d *db.DB, email, role string) string {
	t.Helper()
	var id string
	err := d.Get(&id, `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
VALUES ($1,'U','h',$2,true,false) RETURNING id::text`, email, role)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedInvitation(t *testing.T, d *db.DB, userID string, expires time.Time, accepted *time.Time) {
	t.Helper()
	_, err := d.Exec(`INSERT INTO user_invitations (user_id, invited_by, issued_at, expires_at, accepted_at)
VALUES ($1, $1, $2, $3, $4)`, userID, expires.Add(-72*time.Hour), expires, accepted)
	if err != nil {
		t.Fatalf("seed invitation: %v", err)
	}
}

func seedSession(t *testing.T, d *db.DB, userID string) {
	t.Helper()
	h := make([]byte, 32)
	_, _ = rand.Read(h)
	_, err := d.Exec(`INSERT INTO sessions (user_id, token_hash, created_at, last_seen_at, expires_at) VALUES ($1,$2,$3,$3,$4)`,
		userID, h, N, N.Add(time.Hour))
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func countWhere(t *testing.T, d *db.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.Get(&n, q, args...); err != nil {
		t.Fatal(err)
	}
	return n
}

func userExists(t *testing.T, d *db.DB, id string) bool {
	return countWhere(t, d, `SELECT count(*) FROM users WHERE id=$1`, id) == 1
}

func candidateIDs(t *testing.T, r *Repository) map[string]bool {
	t.Helper()
	cs, err := r.ListCandidates(context.Background(), cutoff, nil, 500)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for _, c := range cs {
		m[c.UserID] = true
	}
	return m
}

func TestRepository_NotCandidates(t *testing.T) {
	d, r := setup(t)
	acc := N.Add(-50 * 24 * time.Hour)
	accepted := seedUser(t, d, "acc@x.id", "developer")
	seedInvitation(t, d, accepted, N.Add(-40*24*time.Hour), &acc)
	plain := seedUser(t, d, "plain@x.id", "developer")
	recent := seedUser(t, d, "recent@x.id", "developer")
	seedInvitation(t, d, recent, cutoff.Add(time.Second), nil)
	fiveDays := seedUser(t, d, "five@x.id", "developer")
	seedInvitation(t, d, fiveDays, N.Add(-5*24*time.Hour), nil)
	exact := seedUser(t, d, "exact@x.id", "developer")
	seedInvitation(t, d, exact, cutoff, nil)

	cands := candidateIDs(t, r)
	if len(cands) != 0 {
		t.Fatalf("kandidat = %v", cands)
	}
	ids := []string{accepted, plain, recent, fiveDays, exact}
	n, err := r.DeleteUsers(context.Background(), ids, cutoff)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	for _, id := range ids {
		if !userExists(t, d, id) {
			t.Fatalf("user %s terhapus", id)
		}
	}
}

func TestRepository_DeleteCandidateCascades(t *testing.T) {
	d, r := setup(t)
	old := seedUser(t, d, "old@x.id", "developer")
	seedInvitation(t, d, old, cutoff.Add(-time.Second), nil)
	seedSession(t, d, old)
	seedSession(t, d, old)
	other := seedUser(t, d, "other@x.id", "developer")
	seedSession(t, d, other)

	if c := candidateIDs(t, r); !c[old] || len(c) != 1 {
		t.Fatalf("kandidat = %v", c)
	}
	n, err := r.DeleteUsers(context.Background(), []string{old}, cutoff)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if userExists(t, d, old) {
		t.Fatal("user masih ada")
	}
	if countWhere(t, d, `SELECT count(*) FROM user_invitations WHERE user_id=$1`, old) != 0 {
		t.Fatal("undangan masih ada")
	}
	if countWhere(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, old) != 0 {
		t.Fatal("sesi masih ada")
	}
	if countWhere(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, other) != 1 {
		t.Fatal("sesi user lain terhapus")
	}
}

func TestRepository_RecheckInSQL(t *testing.T) {
	d, r := setup(t)
	a := seedUser(t, d, "a@x.id", "developer")
	seedInvitation(t, d, a, cutoff.Add(-time.Hour), nil)
	b := seedUser(t, d, "b@x.id", "developer")
	seedInvitation(t, d, b, cutoff.Add(-time.Hour), nil)
	if c := candidateIDs(t, r); !c[a] || !c[b] {
		t.Fatalf("kandidat = %v", c)
	}
	if _, err := d.Exec(`UPDATE user_invitations SET accepted_at=$2 WHERE user_id=$1`, a, N); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE user_invitations SET expires_at=$2 WHERE user_id=$1`, b, N.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	n, err := r.DeleteUsers(context.Background(), []string{a, b}, cutoff)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestRepository_SuperAdminNeverDeleted(t *testing.T) {
	d, r := setup(t)
	sa := seedUser(t, d, "sa@x.id", "super_admin")
	seedInvitation(t, d, sa, cutoff.Add(-time.Hour), nil)
	n, err := r.DeleteUsers(context.Background(), []string{sa}, cutoff)
	if err != nil || n != 0 || !userExists(t, d, sa) {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestRepository_OnlyGivenIDs(t *testing.T) {
	d, r := setup(t)
	a := seedUser(t, d, "a@x.id", "developer")
	seedInvitation(t, d, a, cutoff.Add(-time.Hour), nil)
	b := seedUser(t, d, "b@x.id", "developer")
	seedInvitation(t, d, b, cutoff.Add(-time.Hour), nil)
	n, err := r.DeleteUsers(context.Background(), []string{a}, cutoff)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if userExists(t, d, a) || !userExists(t, d, b) {
		t.Fatal("hanya a yang harus terhapus")
	}
	if n, err := r.DeleteUsers(context.Background(), nil, cutoff); n != 0 || err != nil {
		t.Fatalf("ids kosong: n=%d err=%v", n, err)
	}
}

func TestRepository_KeysetPagination(t *testing.T) {
	d, r := setup(t)
	want := map[string]bool{}
	for i := 0; i < 5; i++ {
		id := seedUser(t, d, string(rune('a'+i))+"@x.id", "developer")
		// expires_at sama untuk sebagian agar tie-break user_id teruji
		seedInvitation(t, d, id, cutoff.Add(-time.Duration(i/2+1)*time.Hour), nil)
		want[id] = true
	}
	got := map[string]bool{}
	var cur *Candidate
	for {
		cs, err := r.ListCandidates(context.Background(), cutoff, cur, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if got[c.UserID] {
				t.Fatalf("duplikat %s", c.UserID)
			}
			got[c.UserID] = true
		}
		if len(cs) < 2 {
			break
		}
		last := cs[len(cs)-1]
		cur = &last
	}
	if len(got) != len(want) {
		t.Fatalf("got %d want %d", len(got), len(want))
	}
}

// Oracle: SQL setuju dengan entity base.Invitation.
func TestRepository_OracleMatchesInvitation(t *testing.T) {
	d, r := setup(t)
	cases := []struct {
		name     string
		expires  time.Time
		accepted *time.Time
	}{
		{"lewat batas", cutoff.Add(-time.Second), nil},
		{"tepat batas", cutoff, nil},
		{"belum lewat", cutoff.Add(time.Second), nil},
		{"diterima", cutoff.Add(-time.Hour), &N},
	}
	cands := map[string]bool{}
	ids := map[string]string{}
	for i, tc := range cases {
		id := seedUser(t, d, string(rune('a'+i))+"@x.id", "developer")
		seedInvitation(t, d, id, tc.expires, tc.accepted)
		ids[tc.name] = id
	}
	cands = candidateIDs(t, r)
	for _, tc := range cases {
		inv := base.Invitation{ExpiresAt: tc.expires, AcceptedAt: tc.accepted}
		want := inv.IsPending() && inv.ExpiresAt.Before(cutoff)
		if cands[ids[tc.name]] != want {
			t.Fatalf("%s: SQL=%v entity=%v", tc.name, cands[ids[tc.name]], want)
		}
	}
}
