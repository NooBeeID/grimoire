package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Querier dipenuhi *sqlx.DB, *sqlx.Tx, dan *DB. Repository menerima Querier
// agar dapat dipakai di dalam maupun di luar transaksi.
type Querier interface {
	sqlx.ExtContext
	GetContext(ctx context.Context, dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
}

var (
	_ Querier = (*sqlx.DB)(nil)
	_ Querier = (*sqlx.Tx)(nil)
	_ Querier = (*DB)(nil)
)

// BeginTx menjalankan fn dalam satu transaksi: commit bila nil, rollback bila
// error (error dikembalikan apa adanya agar errors.Is/As tetap bekerja), dan
// rollback lalu panic diteruskan bila fn panic.
//
// fn tidak boleh memanggil Commit/Rollback sendiri. BeginTx bersarang membuka
// transaksi independen (tidak ada savepoint).
func (d *DB) BeginTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := d.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	done := false
	defer func() {
		if done {
			return
		}
		// fn panic (atau Goexit): rollback, panic tetap naik.
		_ = tx.Rollback()
	}()
	if err = fn(tx); err != nil {
		done = true
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("db: rollback: %w", rbErr))
		}
		return err
	}
	done = true
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}
