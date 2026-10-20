// Package db menyediakan koneksi Postgres, transaksi, dan runner migrasi.
package db

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/infra/config"
)

// DB membungkus pool sqlx di atas pgx stdlib.
type DB struct{ *sqlx.DB }

// Open membuat pool dari cfg dan memastikan DB terjangkau (ping timeout 5 detik).
func Open(ctx context.Context, cfg config.Config) (*DB, error) {
	return open(ctx, cfg, nil)
}

func open(ctx context.Context, cfg config.Config, runtimeParams map[string]string) (*DB, error) {
	connCfg, err := pgx.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		// pgx menyamarkan password di ParseConfigError; jangan menambah URL ke pesan.
		return nil, fmt.Errorf("db: parse DATABASE_URL: %w", err)
	}
	if len(runtimeParams) > 0 {
		if connCfg.RuntimeParams == nil {
			connCfg.RuntimeParams = map[string]string{}
		}
		maps.Copy(connCfg.RuntimeParams, runtimeParams)
	}
	x := sqlx.NewDb(stdlib.OpenDB(*connCfg), "pgx")
	x.SetMaxOpenConns(cfg.DBMaxOpenConns)
	x.SetMaxIdleConns(cfg.DBMaxIdleConns)
	x.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := x.PingContext(pctx); err != nil {
		_ = x.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return &DB{DB: x}, nil
}

// Ping dipakai /readyz. Timeout diatur pemanggil lewat ctx.
func (d *DB) Ping(ctx context.Context) error {
	if err := d.PingContext(ctx); err != nil {
		return fmt.Errorf("db: ping: %w", err)
	}
	return nil
}
