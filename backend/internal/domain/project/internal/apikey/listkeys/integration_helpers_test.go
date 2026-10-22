//go:build integration

package listkeys

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"grimoire/internal/infra/db"
)

const (
	userA = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"
	userB = "0192f5a0-bbbb-7aaa-8bbb-00000000000b"
	userC = "0192f5a0-cccc-7aaa-8bbb-00000000000c"
	userS = "0192f5a0-5555-7aaa-8bbb-000000000005"
)

func setupDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func mkProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

// mkKey menyisipkan key; revokedAt nil = aktif. Mengembalikan id.
func mkKey(t *testing.T, d *db.DB, pid, name, prefix string, created time.Time, revokedAt *time.Time, secretHash []byte) string {
	t.Helper()
	if secretHash == nil {
		h := sha256.Sum256([]byte("s-" + prefix))
		secretHash = h[:]
	}
	var by, label *string
	if revokedAt != nil {
		u, l := "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "a@tim.id"
		by, label = &u, &l
	}
	var id string
	err := d.GetContext(context.Background(), &id, `INSERT INTO api_keys
(project_id, name, prefix, secret_hash, created_at, revoked_at, revoked_by, revoked_by_label)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, pid, name, prefix, secretHash, created, revokedAt, by, label)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// mkOwnedKey seperti mkKey tetapi mengisi created_by dan created_by_label.
func mkOwnedKey(t *testing.T, d *db.DB, pid, name, prefix, ownerID, ownerLabel string, created time.Time, revokedAt *time.Time) string {
	t.Helper()
	h := sha256.Sum256([]byte("s-" + prefix))
	var by, label *string
	if revokedAt != nil {
		u, l := ownerID, ownerLabel
		by, label = &u, &l
	}
	var id string
	err := d.GetContext(context.Background(), &id, `INSERT INTO api_keys
(project_id, name, prefix, secret_hash, created_by, created_by_label, created_at, revoked_at, revoked_by, revoked_by_label)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`,
		pid, name, prefix, h[:], ownerID, ownerLabel, created, revokedAt, by, label)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
