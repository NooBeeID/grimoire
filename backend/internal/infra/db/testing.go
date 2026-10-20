package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"grimoire/internal/infra/config"
)

// TestConfig mengembalikan konfigurasi minimal untuk test integrasi.
// ok=false bila TEST_DATABASE_URL kosong.
func testConfig() (config.Config, bool) {
	url := os.Getenv("TEST_DATABASE_URL")
	return config.Config{
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: time.Minute,
	}, url != ""
}

// TestDB mengembalikan DB terisolasi (schema unik) untuk test integrasi.
// TEST_DATABASE_URL kosong: t.Skip lokal; t.Fatal bila CI=true.
func TestDB(t testing.TB) *DB {
	t.Helper()
	cfg, ok := testConfig()
	if !ok {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL wajib di CI")
		}
		t.Skip("TEST_DATABASE_URL tidak di-set; lewati test integrasi")
	}
	ctx := context.Background()
	admin, err := open(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("db: open admin: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		_ = admin.Close()
		t.Fatalf("db: random schema: %v", err)
	}
	schema := "t_" + hex.EncodeToString(buf)
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		_ = admin.Close()
		t.Fatalf("db: create schema: %v", err)
	}
	d, err := open(ctx, cfg, map[string]string{"search_path": schema})
	if err != nil {
		_, _ = admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close()
		t.Fatalf("db: open test db: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Close()
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("db: drop schema %s: %v", schema, err)
		}
		_ = admin.Close()
	})
	return d
}
