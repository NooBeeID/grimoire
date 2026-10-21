//go:build integration

package store

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/infra/db"
)

func setup(t *testing.T) (*db.DB, *Store, string) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var pid string
	if err := d.GetContext(context.Background(), &pid, `INSERT INTO projects (name) VALUES ('Alpha') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	return d, New(d), pid
}

func hash(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

func insert(d *db.DB, pid, name, prefix string) error {
	_, err := d.ExecContext(context.Background(),
		`INSERT INTO api_keys (project_id, name, prefix, secret_hash) VALUES ($1,$2,$3,$4)`, pid, name, prefix, hash("s"))
	return err
}

func TestStore_GetAndSelect(t *testing.T) {
	d, s, pid := setup(t)
	ctx := context.Background()
	if ks, err := s.SelectKeys(ctx, `SELECT `+KeyColumns+` FROM api_keys WHERE project_id = $1`, pid); err != nil || ks == nil || len(ks) != 0 {
		t.Fatalf("empty: %v %v", ks, err)
	}
	if err := insert(d, pid, "ci", "abcdefgh"); err != nil {
		t.Fatal(err)
	}
	k, ok, err := s.GetKey(ctx, `SELECT `+KeyColumns+` FROM api_keys WHERE prefix = $1`, "abcdefgh")
	if err != nil || !ok {
		t.Fatalf("get: %v %v", ok, err)
	}
	if k.Name != "ci" || k.ProjectID != pid || k.CreatedAt.Location().String() != "UTC" || k.RevokedAt != nil || len(k.SecretHash) != 32 {
		t.Fatalf("%+v", k)
	}
	if _, ok, err := s.GetKey(ctx, `SELECT `+KeyColumns+` FROM api_keys WHERE prefix = $1`, "zzzzzzzz"); ok || err != nil {
		t.Fatalf("missing: %v %v", ok, err)
	}
}

func TestStore_Violation(t *testing.T) {
	d, _, pid := setup(t)
	if err := insert(d, pid, "ci", "abcdefgh"); err != nil {
		t.Fatal(err)
	}
	code, c, ok := Violation(insert(d, pid, "other", "abcdefgh"))
	if !ok || code != CodeUniqueViolation || c != ConstraintPrefixUnique {
		t.Fatalf("prefix: %q %q %v", code, c, ok)
	}
	code, c, ok = Violation(insert(d, pid, "ci", "bbbbbbbb"))
	if !ok || code != CodeUniqueViolation || c != ConstraintActiveName {
		t.Fatalf("name: %q %q %v", code, c, ok)
	}
	code, _, ok = Violation(insert(d, uuid.NewString(), "x1", "cccccccc"))
	if !ok || code != CodeFKViolation {
		t.Fatalf("fk: %q %v", code, ok)
	}
	if _, _, ok := Violation(nil); ok {
		t.Fatal("nil err")
	}
}

func TestStore_Checks(t *testing.T) {
	d, _, pid := setup(t)
	if err := insert(d, pid, "ok", "ABCDEFGH"); err == nil {
		t.Fatal("prefix uppercase harus ditolak")
	}
	if err := insert(d, pid, "x", "dddddddd"); err == nil {
		t.Fatal("nama 1 karakter harus ditolak")
	}
	if _, err := d.Exec(`INSERT INTO api_keys (project_id, name, prefix, secret_hash) VALUES ($1,'ok','eeeeeeee',$2)`, pid, make([]byte, 31)); err == nil {
		t.Fatal("hash 31 byte harus ditolak")
	}
	if _, err := d.Exec(`INSERT INTO api_keys (project_id, name, prefix, secret_hash, revoked_by) VALUES ($1,'ok','ffffffff',$2, gen_random_uuid())`, pid, hash("s")); err == nil {
		t.Fatal("revoked_by tanpa revoked_at harus ditolak")
	}
}

func TestStore_NameReusableAfterRevokeAndCascade(t *testing.T) {
	d, s, pid := setup(t)
	ctx := context.Background()
	if err := insert(d, pid, "ci", "abcdefgh"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE api_keys SET revoked_at = now() WHERE prefix = 'abcdefgh'`); err != nil {
		t.Fatal(err)
	}
	if err := insert(d, pid, "ci", "bbbbbbbb"); err != nil {
		t.Fatalf("nama setelah dicabut: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM projects WHERE id = $1`, pid); err != nil {
		t.Fatal(err)
	}
	ks, err := s.SelectKeys(ctx, `SELECT `+KeyColumns+` FROM api_keys`)
	if err != nil || len(ks) != 0 {
		t.Fatalf("cascade: %v %v", ks, err)
	}
}

func TestValidUUID(t *testing.T) {
	if !ValidUUID(uuid.NewString()) || ValidUUID("bukan-uuid") || ValidUUID("") {
		t.Fatal("ValidUUID")
	}
}
