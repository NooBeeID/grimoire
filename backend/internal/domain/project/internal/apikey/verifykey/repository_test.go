//go:build integration

package verifykey

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/db"
)

func setupDB(t testing.TB) (*db.DB, string) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var pid string
	if err := d.GetContext(context.Background(), &pid, `INSERT INTO projects (name) VALUES ('Alpha') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	return d, pid
}

func insertKey(t testing.TB, d *db.DB, pid, name string, raw base.RawKey, createdBy *string, label string) string {
	t.Helper()
	var id string
	err := d.GetContext(context.Background(), &id,
		`INSERT INTO api_keys (project_id, name, prefix, secret_hash, created_by, created_by_label)
		 VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')) RETURNING id::text`,
		pid, name, raw.Prefix, base.HashSecret(raw.Secret), createdBy, label)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newRaw(t testing.TB) base.RawKey {
	t.Helper()
	r, err := base.GenerateRawKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFindByPrefix_Found(t *testing.T) {
	d, pid := setupDB(t)
	raw := newRaw(t)
	id := insertKey(t, d, pid, "ci", raw, nil, "")
	k, ok, err := newRepo(store.New(d)).FindByPrefix(context.Background(), raw.Prefix)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if k.ID != id || k.ProjectID != pid || k.Name != "ci" || k.Prefix != raw.Prefix || k.RevokedAt != nil || len(k.SecretHash) != 32 {
		t.Fatalf("%+v", k)
	}
	if string(k.SecretHash) != string(base.HashSecret(raw.Secret)) {
		t.Fatal("hash mismatch")
	}
}

func TestFindByPrefix_NotFound(t *testing.T) {
	d, _ := setupDB(t)
	_, ok, err := newRepo(store.New(d)).FindByPrefix(context.Background(), "zzzzzzzz")
	if ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestFindByPrefix_RevokedKeyStillReturned(t *testing.T) {
	d, pid := setupDB(t)
	raw := newRaw(t)
	insertKey(t, d, pid, "ci", raw, nil, "")
	if _, err := d.Exec(`UPDATE api_keys SET revoked_at = now() WHERE prefix = $1`, raw.Prefix); err != nil {
		t.Fatal(err)
	}
	k, ok, err := newRepo(store.New(d)).FindByPrefix(context.Background(), raw.Prefix)
	if err != nil || !ok || k.RevokedAt == nil || k.RevokedAt.Location() != time.UTC {
		t.Fatalf("ok=%v err=%v k=%+v", ok, err, k)
	}
}

func TestFindByPrefix_ContextCanceled(t *testing.T) {
	d, pid := setupDB(t)
	raw := newRaw(t)
	insertKey(t, d, pid, "ci", raw, nil, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, ok, err := newRepo(store.New(d)).FindByPrefix(ctx, raw.Prefix)
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestFindByPrefix_UsesPrefixIndex(t *testing.T) {
	d, pid := setupDB(t)
	insertKey(t, d, pid, "ci", newRaw(t), nil, "")
	ctx := context.Background()
	tx, err := d.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	var plan string
	if err := tx.GetContext(ctx, &plan, `EXPLAIN (FORMAT JSON) `+strings.Replace(qFindByPrefix, "$1", "'abcdefgh'", 1)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "api_keys_prefix_key") || strings.Contains(plan, "Seq Scan") {
		t.Fatalf("plan: %s", plan)
	}
}

func TestFindByPrefix_SQLParameterized(t *testing.T) {
	d, pid := setupDB(t)
	insertKey(t, d, pid, "ci", newRaw(t), nil, "")
	_, ok, err := newRepo(store.New(d)).FindByPrefix(context.Background(), `' OR '1'='1`)
	if ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	var n int
	if err := d.Get(&n, `SELECT count(*) FROM api_keys`); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
