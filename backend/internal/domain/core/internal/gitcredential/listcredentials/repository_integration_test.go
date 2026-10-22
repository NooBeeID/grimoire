//go:build integration

package listcredentials

import (
	"context"
	"fmt"
	"testing"
	"time"

	"grimoire/internal/infra/db"
)

func setupDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func mkUser(t *testing.T, d *db.DB, email string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id,
		`INSERT INTO users (email,name,password_hash,role) VALUES ($1,'U','x','developer') RETURNING id::text`, email); err != nil {
		t.Fatal(err)
	}
	return id
}

func mkCred(t *testing.T, d *db.DB, userID, provider, host, last4 string, created, updated time.Time) {
	t.Helper()
	enc := append([]byte("CANARY"), make([]byte, 34)...)
	if _, err := d.ExecContext(context.Background(), `INSERT INTO git_credentials
(user_id, provider, host, token_enc, last4, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		userID, provider, host, enc, last4, created, updated); err != nil {
		t.Fatal(err)
	}
}

func TestRepo_ListByUser(t *testing.T) {
	d := setupDB(t)
	r := newRepo(d)
	a, b := mkUser(t, d, "a@tim.id"), mkUser(t, d, "b@tim.id")
	c1 := time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC)
	c2 := c1.Add(time.Hour)
	mkCred(t, d, a, "gitlab", "z.example.com", "zzzz", c1, c2)
	mkCred(t, d, a, "github", "github.com", "aaaa", c1, c2)
	mkCred(t, d, a, "gitlab", "a.example.com", "bbbb", c1, c2)
	mkCred(t, d, a, "gitlab", "gitlab.com", "cccc", c1, c2)
	mkCred(t, d, b, "gitlab", "gitlab.com", "BBBB", c1, c2)

	got, err := r.ListByUser(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"github/github.com", "gitlab/a.example.com", "gitlab/gitlab.com", "gitlab/z.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %d", len(got))
	}
	for i, c := range got {
		if string(c.Provider)+"/"+c.Host != want[i] || c.TokenEnc != nil || c.Last4 == "BBBB" {
			t.Fatalf("row %d = %+v", i, c)
		}
		if c.CreatedAt.Location() != time.UTC || !c.CreatedAt.Equal(c1) || !c.UpdatedAt.Equal(c2) {
			t.Fatalf("time %+v", c)
		}
		if s := fmt.Sprintf("%+v", c); contains(s, "CANARY") {
			t.Fatal("token bocor")
		}
	}
	gb, _ := r.ListByUser(context.Background(), b)
	if len(gb) != 1 || gb[0].Last4 != "BBBB" {
		t.Fatalf("b=%+v", gb)
	}
}

func TestRepo_EmptyInvalidAndCascade(t *testing.T) {
	d := setupDB(t)
	r := newRepo(d)
	u := mkUser(t, d, "c@tim.id")
	for _, id := range []string{u, "bukan-uuid", "0192f5a0-aaaa-7aaa-8bbb-00000000ffff"} {
		got, err := r.ListByUser(context.Background(), id)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("id=%s got=%v err=%v", id, got, err)
		}
	}
	now := time.Now()
	mkCred(t, d, u, "github", "github.com", "1234", now, now)
	if _, err := d.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, u); err != nil {
		t.Fatal(err)
	}
	got, _ := r.ListByUser(context.Background(), u)
	if len(got) != 0 {
		t.Fatalf("cascade gagal: %v", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
