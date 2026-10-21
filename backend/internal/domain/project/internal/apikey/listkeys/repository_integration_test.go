//go:build integration

package listkeys

import (
	"context"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base/store"
)

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

	got, err := newRepo(store.New(d)).ListByProject(context.Background(), p)
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
	got, err := r.ListByProject(context.Background(), p1)
	if err != nil || len(got) != 1 || got[0].Name != "k1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	got, err = r.ListByProject(context.Background(), p3)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %+v %v", got, err)
	}
	got, err = r.ListByProject(context.Background(), "bukan-uuid")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("non-uuid: %+v %v", got, err)
	}
}

func TestRepo_DoesNotReadSecretHash(t *testing.T) {
	if strings.Contains(listByProjectSQL, "secret_hash") {
		t.Fatal("query must not select secret_hash")
	}
	d := setupDB(t)
	p := mkProject(t, d, "P1")
	h := make([]byte, 32)
	h[0] = 7
	mkKey(t, d, p, "k1", "ccccccc1", time.Now(), nil, h)
	got, err := newRepo(store.New(d)).ListByProject(context.Background(), p)
	if err != nil || len(got) != 1 || len(got[0].SecretHash) != 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
