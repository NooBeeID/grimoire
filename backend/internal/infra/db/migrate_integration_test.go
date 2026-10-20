//go:build integration

package db

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

const (
	v1 = "20261020080000001"
	v2 = "20261020080000002"
	v3 = "20261020080000003"
)

func mapFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func tableExists(t *testing.T, d *DB, name string) bool {
	t.Helper()
	var ok bool
	if err := d.Get(&ok, "SELECT to_regclass($1) IS NOT NULL", name); err != nil {
		t.Fatal(err)
	}
	return ok
}

func appliedVersions(t *testing.T, d *DB) []int64 {
	t.Helper()
	var vs []int64
	if err := d.Select(&vs, "SELECT version FROM schema_migrations ORDER BY version"); err != nil {
		t.Fatal(err)
	}
	return vs
}

func advisoryLocks(t *testing.T, d *DB) int {
	t.Helper()
	var n int
	if err := d.Get(&n, "SELECT count(*) FROM pg_locks WHERE locktype='advisory'"); err != nil {
		t.Fatal(err)
	}
	return n
}

func baseFS() fstest.MapFS {
	return mapFS(map[string]string{
		"core/" + v1 + "_a.sql":    "CREATE TABLE a(id int); INSERT INTO a VALUES (1);",
		"project/" + v2 + "_b.sql": "CREATE TABLE b(id int);",
	})
}

func TestT00_AC5_MigrateTwiceNoEffect(t *testing.T) {
	d := TestDB(t)
	ctx := context.Background()
	res, err := Migrate(ctx, d, baseFS(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 2 || res.Applied[0] != "core/"+v1+"_a.sql" || res.Applied[1] != "project/"+v2+"_b.sql" {
		t.Fatalf("applied=%v", res.Applied)
	}
	var before []time.Time
	if err := d.Select(&before, "SELECT applied_at FROM schema_migrations ORDER BY version"); err != nil {
		t.Fatal(err)
	}
	res, err = Migrate(ctx, d, baseFS(), nil)
	if err != nil || len(res.Applied) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	var cnt int
	_ = d.Get(&cnt, "SELECT count(*) FROM a")
	if cnt != 1 {
		t.Fatalf("a count=%d", cnt)
	}
	var after []time.Time
	_ = d.Select(&after, "SELECT applied_at FROM schema_migrations ORDER BY version")
	if len(after) != 2 || !after[0].Equal(before[0]) || !after[1].Equal(before[1]) {
		t.Fatalf("before=%v after=%v", before, after)
	}
}

func TestT00_AC5_FailedFileRollsBack(t *testing.T) {
	d := TestDB(t)
	ctx := context.Background()
	fsys := mapFS(map[string]string{
		"core/" + v1 + "_a.sql":    "CREATE TABLE a(id int);",
		"core/" + v2 + "_b.sql":    "CREATE TABLE b(id int); SELECT 1/0;",
		"project/" + v3 + "_c.sql": "CREATE TABLE c(id int);",
	})
	res, err := Migrate(ctx, d, fsys, nil)
	if err == nil || !strings.Contains(err.Error(), "core/"+v2+"_b.sql") || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("err=%v", err)
	}
	if len(res.Applied) != 1 || res.Applied[0] != "core/"+v1+"_a.sql" {
		t.Fatalf("applied=%v", res.Applied)
	}
	if tableExists(t, d, "b") || tableExists(t, d, "c") || !tableExists(t, d, "a") {
		t.Fatal("state tabel salah")
	}
	if vs := appliedVersions(t, d); len(vs) != 1 {
		t.Fatalf("versions=%v", vs)
	}
	if n := advisoryLocks(t, d); n != 0 {
		t.Fatalf("lock bocor: %d", n)
	}
	fsys["core/"+v2+"_b.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE b(id int);")}
	res, err = Migrate(ctx, d, fsys, nil)
	if err != nil || len(res.Applied) != 2 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if !tableExists(t, d, "a") || !tableExists(t, d, "b") || !tableExists(t, d, "c") || len(appliedVersions(t, d)) != 3 {
		t.Fatal("state akhir salah")
	}
}

func TestT00_AC5_InvalidAndDuplicateRejectedWithoutSideEffects(t *testing.T) {
	d := TestDB(t)
	ctx := context.Background()
	_, err := Migrate(ctx, d, mapFS(map[string]string{
		"core/" + v1 + "_a.sql": "CREATE TABLE a(id int);",
		"core/bad.sql":          "SELECT 1;",
	}), nil)
	if !errors.Is(err, ErrInvalidMigrationName) {
		t.Fatalf("err=%v", err)
	}
	_, err = Migrate(ctx, d, mapFS(map[string]string{
		"core/" + v1 + "_a.sql":    "CREATE TABLE a(id int);",
		"project/" + v1 + "_b.sql": "CREATE TABLE b(id int);",
	}), nil)
	if !errors.Is(err, ErrDuplicateVersion) {
		t.Fatalf("err=%v", err)
	}
	if tableExists(t, d, "schema_migrations") || tableExists(t, d, "a") || tableExists(t, d, "b") {
		t.Fatal("ada efek samping")
	}
}

func TestT00_AC5_ConcurrentRunners(t *testing.T) {
	d := TestDB(t)
	fsys := mapFS(map[string]string{
		"core/" + v1 + "_a.sql":    "SELECT pg_sleep(0.3); CREATE TABLE a(id int);",
		"core/" + v2 + "_b.sql":    "CREATE TABLE b(id int);",
		"project/" + v3 + "_c.sql": "CREATE TABLE c(id int);",
	})
	var wg sync.WaitGroup
	results := make([]MigrateResult, 2)
	errsOut := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errsOut[i] = Migrate(context.Background(), d, fsys, nil)
		}()
	}
	wg.Wait()
	for _, e := range errsOut {
		if e != nil {
			t.Fatal(e)
		}
	}
	if len(results[0].Applied)+len(results[1].Applied) != 3 {
		t.Fatalf("applied=%v %v", results[0].Applied, results[1].Applied)
	}
	if vs := appliedVersions(t, d); len(vs) != 3 {
		t.Fatalf("versions=%v", vs)
	}
}

func TestMigrate_WaitsWhileLockHeld(t *testing.T) {
	d := TestDB(t)
	ctx := context.Background()
	holder, err := d.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if _, err := holder.ExecContext(ctx, lockSQL); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	_, err = Migrate(cctx, d, baseFS(), nil)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		// pgx dapat melaporkan pembatalan sebagai error server (57014); terima keduanya.
		if err == nil || !strings.Contains(err.Error(), "57014") && !strings.Contains(err.Error(), "deadline") {
			t.Fatalf("err=%v", err)
		}
	}
	if _, err := holder.ExecContext(ctx, unlockSQL); err != nil {
		t.Fatal(err)
	}
	if tableExists(t, d, "a") {
		t.Fatal("tabel dibuat saat lock ditahan")
	}
	if _, err := Migrate(ctx, d, baseFS(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestMigrate_SingleConnPool(t *testing.T) {
	cfg, _ := testConfig()
	cfg.DBMaxOpenConns = 1
	cfg.DBMaxIdleConns = 1
	ctx := context.Background()
	admin := TestDB(t) // pastikan skip/env
	_ = admin
	d, err := open(ctx, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec("DROP TABLE IF EXISTS a, b, schema_migrations")
		_ = d.Close()
	})
	_, _ = d.Exec("DROP TABLE IF EXISTS a, b, schema_migrations")
	done := make(chan error, 1)
	go func() { _, err := Migrate(ctx, d, baseFS(), nil); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("deadlock")
	}
}

func TestMigrate_CreatesSchemaMigrationsOnEmptyFS(t *testing.T) {
	d := TestDB(t)
	res, err := Migrate(context.Background(), d, fstest.MapFS{}, nil)
	if err != nil || len(res.Applied) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if !tableExists(t, d, "schema_migrations") {
		t.Fatal("schema_migrations tidak ada")
	}
}

func TestMigrate_UnknownDBVersionWarns(t *testing.T) {
	d := TestDB(t)
	ctx := context.Background()
	if _, err := Migrate(ctx, d, fstest.MapFS{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec("INSERT INTO schema_migrations(version, name) VALUES (20261101000000000, 'x.sql')"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	if _, err := Migrate(ctx, d, baseFS(), log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "migration version in database but not in binary") {
		t.Fatalf("log=%s", buf.String())
	}
}

func TestMigrate_SkipsAlreadyAppliedAndRunsNewOnly(t *testing.T) {
	d := TestDB(t)
	ctx := context.Background()
	fsys := mapFS(map[string]string{"core/" + v1 + "_a.sql": "CREATE TABLE a(id int);"})
	if _, err := Migrate(ctx, d, fsys, nil); err != nil {
		t.Fatal(err)
	}
	fsys["core/"+v2+"_b.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE b(id int);")}
	res, err := Migrate(ctx, d, fsys, nil)
	if err != nil || len(res.Applied) != 1 || res.Applied[0] != "core/"+v2+"_b.sql" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestMigrate_ConnectionReturnedClean(t *testing.T) {
	d := TestDB(t)
	ctx := context.Background()
	if _, err := Migrate(ctx, d, baseFS(), nil); err != nil {
		t.Fatal(err)
	}
	other, err := d.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var got bool
	if err := other.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtext('grimoire.migrate'))").Scan(&got); err != nil || !got {
		t.Fatalf("got=%v err=%v", got, err)
	}
	_, _ = other.ExecContext(ctx, unlockSQL)
}

func TestUp_EmbeddedFS(t *testing.T) {
	d := TestDB(t)
	res, err := Up(context.Background(), d, nil)
	wantApplied := []string{"core/20261021090000000_create_audit_log.sql", "core/20261021100000000_create_auth_tables.sql"}
	if err != nil || !reflect.DeepEqual(res.Applied, wantApplied) {
		t.Fatalf("run 1: res=%v err=%v, want %v", res, err, wantApplied)
	}
	res, err = Up(context.Background(), d, nil)
	if err != nil || len(res.Applied) != 0 {
		t.Fatalf("run 2 (idempoten): res=%v err=%v", res, err)
	}
	if res2, err := Up(context.Background(), d, nil); err != nil || len(res2.Applied) != 0 {
		t.Fatalf("idempoten: res=%v err=%v", res2, err)
	}
}

func TestMigrate_MultiStatementAndInsertAtomic(t *testing.T) {
	d := TestDB(t)
	ctx := context.Background()
	res, err := Migrate(ctx, d, mapFS(map[string]string{
		"core/" + v1 + "_a.sql": "CREATE TABLE a(id int); CREATE INDEX a_id ON a(id); INSERT INTO a VALUES (1);",
	}), nil)
	if err != nil || len(res.Applied) != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	_, err = Migrate(ctx, d, mapFS(map[string]string{
		"core/" + v1 + "_a.sql": "CREATE TABLE a(id int); CREATE INDEX a_id ON a(id); INSERT INTO a VALUES (1);",
		"core/" + v2 + "_b.sql": "CREATE TABLE b(id int); INSERT INTO nonexistent VALUES (1);",
	}), nil)
	if err == nil {
		t.Fatal("harus error")
	}
	if vs := appliedVersions(t, d); len(vs) != 1 || tableExists(t, d, "b") {
		t.Fatalf("versions=%v", vs)
	}
}

func TestMigrate_ContextCancelledMidRun(t *testing.T) {
	d := TestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(400 * time.Millisecond); cancel() }()
	_, err := Migrate(ctx, d, mapFS(map[string]string{
		"core/" + v1 + "_a.sql": "CREATE TABLE a(id int); SELECT pg_sleep(2);",
	}), nil)
	if err == nil {
		t.Fatal("harus error")
	}
	if tableExists(t, d, "a") || len(appliedVersions(t, d)) != 0 {
		t.Fatal("state tersisa")
	}
	deadline := time.Now().Add(3 * time.Second)
	for advisoryLocks(t, d) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("lock bocor")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// --- transaksi ---
