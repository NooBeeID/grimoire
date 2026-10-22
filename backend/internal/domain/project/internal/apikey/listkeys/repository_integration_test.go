//go:build integration

package listkeys

import (
	"context"
	"strings"
	"testing"
	"time"

	basepkg "grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
)

var all = basepkg.OwnerScope{All: true}

func TestRepo_OrderAndRevoked(t *testing.T) {
	d := setupDB(t)
	p := mkProject(t, d, "P1")
	base := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	rev := base.Add(5 * time.Hour)
	a := mkKey(t, d, p, "kA", "aaaaaaa1", base, nil, nil)
	b := mkKey(t, d, p, "kB", "aaaaaaa2", base.Add(time.Hour), nil, nil)
	c := mkKey(t, d, p, "kC", "aaaaaaa3", base.Add(2*time.Hour), &rev, nil)
	dd := mkKey(t, d, p, "kD", "aaaaaaa4", base.Add(-2*time.Hour), &rev, nil)
	e := mkKey(t, d, p, "kE", "aaaaaaa5", base, nil, nil)

	got, err := newRepo(store.New(d)).ListByProject(context.Background(), p, all)
	if err != nil || len(got) != 5 {
		t.Fatalf("got=%d err=%v", len(got), err)
	}
	lo, hi := a, e
	if lo > hi {
		lo, hi = hi, lo
	}
	want := []string{b, lo, hi, c, dd}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("pos %d: got %s want %s", i, got[i].ID, id)
		}
	}
	if got[3].RevokedAt == nil || got[3].RevokedBy == nil || got[3].RevokedByLabel == nil {
		t.Fatalf("revoked fields: %+v", got[3])
	}
	if got[0].CreatedAt.Location() != time.UTC || got[3].RevokedAt.Location() != time.UTC {
		t.Fatal("not UTC")
	}
	if got[0].CreatedBy != nil || got[0].CreatedByLabel != nil || got[0].LastUsedAt != nil {
		t.Fatalf("nullable: %+v", got[0])
	}
}

func TestRepo_FiltersByProjectAndEmpty(t *testing.T) {
	d := setupDB(t)
	p1, p2, p3 := mkProject(t, d, "P1"), mkProject(t, d, "P2"), mkProject(t, d, "P3")
	mkKey(t, d, p1, "k1", "bbbbbbb1", time.Now(), nil, nil)
	mkKey(t, d, p2, "k2", "bbbbbbb2", time.Now(), nil, nil)
	r := newRepo(store.New(d))
	got, err := r.ListByProject(context.Background(), p1, all)
	if err != nil || len(got) != 1 || got[0].Name != "k1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	got, err = r.ListByProject(context.Background(), p3, all)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %+v %v", got, err)
	}
	got, err = r.ListByProject(context.Background(), "bukan-uuid", all)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("non-uuid: %+v %v", got, err)
	}
}

func TestRepo_DoesNotReadSecretHash(t *testing.T) {
	if strings.Contains(listByProjectSQL, "secret_hash") || strings.Contains(listByOwnerSQL, "secret_hash") {
		t.Fatal("query must not select secret_hash")
	}
	d := setupDB(t)
	p := mkProject(t, d, "P1")
	h := make([]byte, 32)
	h[0] = 7
	mkKey(t, d, p, "k1", "ccccccc1", time.Now(), nil, h)
	for _, sc := range []basepkg.OwnerScope{all, {OwnerID: userA}} {
		got, err := newRepo(store.New(d)).ListByProject(context.Background(), p, sc)
		if sc.All && (err != nil || len(got) != 1 || len(got[0].SecretHash) != 0) {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func names(ks []basepkg.Key) []string {
	out := make([]string, 0, len(ks))
	for _, k := range ks {
		out = append(out, k.Name)
	}
	return out
}

func sameSet(t *testing.T, got []basepkg.Key, want ...string) {
	t.Helper()
	g := names(got)
	if len(g) != len(want) {
		t.Fatalf("got %v want %v", g, want)
	}
	m := map[string]bool{}
	for _, n := range g {
		m[n] = true
	}
	for _, w := range want {
		if !m[w] {
			t.Fatalf("got %v want %v", g, want)
		}
	}
}

func seedOwners(t *testing.T) (*store.Store, string, string) {
	d := setupDB(t)
	p1, p2 := mkProject(t, d, "P1"), mkProject(t, d, "P2")
	base := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	rev := base.Add(time.Hour)
	mkOwnedKey(t, d, p1, "a1", "eeeeeee1", userA, "a@tim.id", base, nil)
	mkOwnedKey(t, d, p1, "a2", "eeeeeee2", userA, "a@tim.id", base.Add(time.Minute), &rev)
	mkOwnedKey(t, d, p1, "b1", "eeeeeee3", userB, "b@tim.id", base, nil)
	mkKey(t, d, p1, "legacy", "eeeeeee4", base, nil, nil)
	mkOwnedKey(t, d, p2, "a-p2", "eeeeeee5", userA, "a@tim.id", base, nil)
	return store.New(d), p1, p2
}

func TestRepo_OwnerIsolation(t *testing.T) {
	s, p1, p2 := seedOwners(t)
	r := newRepo(s)
	ctx := context.Background()
	got, err := r.ListByProject(ctx, p1, basepkg.OwnerScope{OwnerID: userA})
	if err != nil {
		t.Fatal(err)
	}
	sameSet(t, got, "a1", "a2")
	if got[0].Name != "a1" || got[1].Name != "a2" || got[1].RevokedAt == nil || got[0].CreatedBy == nil || *got[0].CreatedBy != userA || got[0].CreatedByLabel == nil {
		t.Fatalf("fields/order: %+v", got)
	}
	got, _ = r.ListByProject(ctx, p1, basepkg.OwnerScope{OwnerID: userB})
	sameSet(t, got, "b1")
	got, _ = r.ListByProject(ctx, p1, basepkg.OwnerScope{OwnerID: userC})
	if got == nil || len(got) != 0 {
		t.Fatalf("userC: %+v", got)
	}
	got, _ = r.ListByProject(ctx, p1, all)
	sameSet(t, got, "a1", "a2", "b1", "legacy")
	got, _ = r.ListByProject(ctx, p2, all)
	sameSet(t, got, "a-p2")
	got, _ = r.ListByProject(ctx, p2, basepkg.OwnerScope{OwnerID: userA})
	sameSet(t, got, "a-p2")
}

func TestRepo_OwnerOrdering(t *testing.T) {
	d := setupDB(t)
	p := mkProject(t, d, "P1")
	t0 := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	rev := t0.Add(5 * time.Hour)
	mk := func(n, pre string, c time.Time, r *time.Time) string {
		return mkOwnedKey(t, d, p, n, pre, userA, "a@tim.id", c, r)
	}
	a := mk("kA", "fffffff1", t0, nil)
	b := mk("kB", "fffffff2", t0.Add(time.Hour), nil)
	c := mk("kC", "fffffff3", t0.Add(2*time.Hour), &rev)
	dd := mk("kD", "fffffff4", t0.Add(-2*time.Hour), &rev)
	e := mk("kE", "fffffff5", t0, nil)
	lo, hi := a, e
	if lo > hi {
		lo, hi = hi, lo
	}
	want := []string{b, lo, hi, c, dd}
	for _, sc := range []basepkg.OwnerScope{{OwnerID: userA}, all} {
		got, err := newRepo(store.New(d)).ListByProject(context.Background(), p, sc)
		if err != nil || len(got) != 5 {
			t.Fatalf("scope=%+v got=%d err=%v", sc, len(got), err)
		}
		for i, id := range want {
			if got[i].ID != id {
				t.Fatalf("scope=%+v pos %d: got %s want %s", sc, i, got[i].ID, id)
			}
		}
	}
}

func TestRepo_FailClosed(t *testing.T) {
	s, p1, _ := seedOwners(t)
	r := newRepo(s)
	cases := map[string]struct {
		pid string
		sc  basepkg.OwnerScope
	}{
		"scope kosong":           {p1, basepkg.OwnerScope{}},
		"owner bukan uuid":       {p1, basepkg.OwnerScope{OwnerID: "bukan-uuid"}},
		"project bukan uuid":     {"bukan-uuid", basepkg.OwnerScope{OwnerID: userA}},
		"project bukan uuid all": {"bukan-uuid", all},
	}
	for n, c := range cases {
		got, err := r.ListByProject(context.Background(), c.pid, c.sc)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("%s: got=%+v err=%v", n, got, err)
		}
	}
}

func TestRepo_AllWinsOverOwnerID(t *testing.T) {
	s, p1, _ := seedOwners(t)
	got, err := newRepo(s).ListByProject(context.Background(), p1, basepkg.OwnerScope{All: true, OwnerID: userA})
	if err != nil {
		t.Fatal(err)
	}
	sameSet(t, got, "a1", "a2", "b1", "legacy")
}
