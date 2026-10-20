//go:build integration

package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/infra/config"
	"grimoire/lib/common/errs"
)

func txTable(t *testing.T, d *DB) {
	t.Helper()
	if _, err := d.Exec("CREATE TABLE tx_t(id int)"); err != nil {
		t.Fatal(err)
	}
}

func txCount(t *testing.T, q Querier) int {
	t.Helper()
	var n int
	if err := q.GetContext(context.Background(), &n, "SELECT count(*) FROM tx_t"); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBeginTx_Commit(t *testing.T) {
	d := TestDB(t)
	txTable(t, d)
	err := d.BeginTx(context.Background(), func(tx *sqlx.Tx) error {
		_, err := tx.Exec("INSERT INTO tx_t VALUES (1)")
		return err
	})
	if err != nil || txCount(t, d) != 1 {
		t.Fatalf("err=%v", err)
	}
}

func TestBeginTx_RollbackOnError(t *testing.T) {
	d := TestDB(t)
	txTable(t, d)
	err := d.BeginTx(context.Background(), func(tx *sqlx.Tx) error {
		if _, err := tx.Exec("INSERT INTO tx_t VALUES (1)"); err != nil {
			return err
		}
		return errs.ErrConflict
	})
	if !errors.Is(err, errs.ErrConflict) || txCount(t, d) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestBeginTx_RollbackAndRepanic(t *testing.T) {
	d := TestDB(t)
	txTable(t, d)
	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Fatalf("recover=%v", r)
			}
		}()
		_ = d.BeginTx(context.Background(), func(tx *sqlx.Tx) error {
			_, _ = tx.Exec("INSERT INTO tx_t VALUES (1)")
			panic("boom")
		})
	}()
	if txCount(t, d) != 0 || d.Stats().InUse != 0 {
		t.Fatalf("inuse=%d", d.Stats().InUse)
	}
}

func TestBeginTx_CommitError(t *testing.T) {
	d := TestDB(t)
	err := d.BeginTx(context.Background(), func(tx *sqlx.Tx) error { return tx.Commit() })
	if !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("err=%v", err)
	}
}

func TestBeginTx_RollbackErrorJoined(t *testing.T) {
	d := TestDB(t)
	sentinel := errors.New("biz")
	err := d.BeginTx(context.Background(), func(tx *sqlx.Tx) error {
		_ = tx.Rollback()
		return sentinel
	})
	if !errors.Is(err, sentinel) || errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("err=%v", err)
	}
}

func TestBeginTx_ContextCancelled(t *testing.T) {
	d := TestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := d.BeginTx(ctx, func(tx *sqlx.Tx) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestBeginTx_QuerierAcceptsTx(t *testing.T) {
	d := TestDB(t)
	txTable(t, d)
	if txCount(t, d) != 0 {
		t.Fatal("db")
	}
	_ = d.BeginTx(context.Background(), func(tx *sqlx.Tx) error {
		if txCount(t, tx) != 0 {
			t.Error("tx")
		}
		return nil
	})
}

// --- koneksi ---

func TestOpen_PoolSettings(t *testing.T) {
	cfg, ok := testConfig()
	if !ok {
		TestDB(t)
	}
	cfg.DBMaxOpenConns = 7
	d, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Stats().MaxOpenConnections != 7 {
		t.Fatalf("max=%d", d.Stats().MaxOpenConnections)
	}
}

func TestOpen_UnreachableFailsFast(t *testing.T) {
	start := time.Now()
	_, err := Open(context.Background(), config.Config{
		DatabaseURL: "postgres://u:pw@127.0.0.1:1/db?sslmode=disable", DBMaxOpenConns: 1, DBMaxIdleConns: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "db: ping") || time.Since(start) > 6*time.Second {
		t.Fatalf("err=%v elapsed=%v", err, time.Since(start))
	}
}

func TestPing_OK(t *testing.T) {
	d := TestDB(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPing_AfterClose(t *testing.T) {
	cfg, ok := testConfig()
	if !ok {
		TestDB(t)
	}
	d, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = d.Close()
	if err := d.Ping(context.Background()); err == nil {
		t.Fatal("harus error")
	}
}

func TestTestDB_IsolatesSchemas(t *testing.T) {
	a := TestDB(t)
	b := TestDB(t)
	if _, err := a.Exec("CREATE TABLE only_a(id int)"); err != nil {
		t.Fatal(err)
	}
	if tableExists(t, b, "only_a") {
		t.Fatal("schema bocor")
	}
}
