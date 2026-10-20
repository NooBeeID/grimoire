package db

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

var (
	ErrInvalidMigrationName = errors.New("db: invalid migration file name")
	ErrDuplicateVersion     = errors.New("db: duplicate migration version")
)

// MigrateResult berisi path relatif FS file yang berhasil diterapkan, urut eksekusi.
type MigrateResult struct{ Applied []string }

type migration struct {
	Version int64
	Name    string
	Path    string
}

var migrationNameRE = regexp.MustCompile(`^(\d{17})_[a-z0-9_]+\.sql$`)

const (
	lockSQL   = "SELECT pg_advisory_lock(hashtext('grimoire.migrate'))"
	unlockSQL = "SELECT pg_advisory_unlock(hashtext('grimoire.migrate'))"
)

// plan membaca, memvalidasi, dan mengurutkan migrasi. Murni, tanpa DB.
func plan(fsys fs.FS) ([]migration, error) {
	var list []migration
	seen := map[int64]string{}
	err := fs.WalkDir(fsys, ".", func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !strings.HasSuffix(p, ".sql") {
			return nil
		}
		base := e.Name()
		m := migrationNameRE.FindStringSubmatch(base)
		if m == nil {
			return fmt.Errorf("%w: %s (harus ^\\d{17}_[a-z0-9_]+\\.sql$)", ErrInvalidMigrationName, p)
		}
		v, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalidMigrationName, p, err)
		}
		if prev, ok := seen[v]; ok {
			return fmt.Errorf("%w: %d di %s dan %s", ErrDuplicateVersion, v, prev, p)
		}
		seen[v] = p
		list = append(list, migration{Version: v, Name: base, Path: p})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidMigrationName) || errors.Is(err, ErrDuplicateVersion) {
			return nil, err
		}
		return nil, fmt.Errorf("db: read migrations: %w", err)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Version < list[j].Version })
	return list, nil
}

// Migrate menerapkan migrasi dari fsys yang belum tercatat di schema_migrations.
// Hasil parsial (file yang sudah commit) dikembalikan bersama error.
func Migrate(ctx context.Context, d *DB, fsys fs.FS, log *slog.Logger) (MigrateResult, error) {
	if log == nil {
		log = slog.Default()
	}
	list, err := plan(fsys)
	if err != nil {
		return MigrateResult{}, err
	}
	conn, err := d.DB.Conn(ctx)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("db: migrate: acquire conn: %w", err)
	}
	defer conn.Close()

	var (
		res     MigrateResult
		badConn bool
		runErr  error
		simple  = pgx.QueryExecModeSimpleProtocol
		bgCtx   = context.WithoutCancel(ctx)
		rawErr  error
	)
	rawErr = conn.Raw(func(driverConn any) error {
		sc, ok := driverConn.(*stdlib.Conn)
		if !ok {
			runErr = errors.New("db: migrate requires pgx connection")
			return nil
		}
		pc := sc.Conn()

		if _, err := pc.Exec(ctx, lockSQL, simple); err != nil {
			runErr = fmt.Errorf("db: acquire migration lock: %w", err)
			// Permintaan lock yang dibatalkan bisa meninggalkan koneksi tak pasti.
			badConn = true
			return nil
		}
		defer func() {
			uctx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
			defer cancel()
			var released bool
			if err := pc.QueryRow(uctx, unlockSQL, simple).Scan(&released); err != nil || !released {
				log.ErrorContext(ctx, "failed to release migration lock", "error", err, "released", released)
				badConn = true
			}
		}()

		if _, err := pc.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			version    bigint      PRIMARY KEY,
			name       text        NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`, simple); err != nil {
			runErr = fmt.Errorf("db: create schema_migrations: %w", err)
			return nil
		}

		rows, err := pc.Query(ctx, "SELECT version FROM schema_migrations", simple)
		if err != nil {
			runErr = fmt.Errorf("db: read schema_migrations: %w", err)
			return nil
		}
		installed := map[int64]struct{}{}
		for rows.Next() {
			var v int64
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				runErr = fmt.Errorf("db: scan schema_migrations: %w", err)
				return nil
			}
			installed[v] = struct{}{}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			runErr = fmt.Errorf("db: read schema_migrations: %w", err)
			return nil
		}
		known := make(map[int64]struct{}, len(list))
		for _, m := range list {
			known[m.Version] = struct{}{}
		}
		unknown := make([]int64, 0)
		for v := range installed {
			if _, ok := known[v]; !ok {
				unknown = append(unknown, v)
			}
		}
		sort.Slice(unknown, func(i, j int) bool { return unknown[i] < unknown[j] })
		for _, v := range unknown {
			log.WarnContext(ctx, "migration version in database but not in binary", "version", v)
		}

		for _, m := range list {
			if _, ok := installed[m.Version]; ok {
				continue
			}
			start := time.Now()
			body, err := fs.ReadFile(fsys, m.Path)
			if err != nil {
				runErr = fmt.Errorf("migration %s: %w", m.Path, err)
				return nil
			}
			tx, err := pc.Begin(ctx)
			if err != nil {
				runErr = fmt.Errorf("migration %s: begin: %w", m.Path, err)
				return nil
			}
			rollback := func() {
				rctx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
				defer cancel()
				if err := tx.Rollback(rctx); err != nil {
					badConn = true
				}
			}
			if _, err := tx.Exec(ctx, string(body), simple); err != nil {
				rollback()
				runErr = fmt.Errorf("migration %s: %w", m.Path, err)
				return nil
			}
			if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version, name) VALUES ($1, $2)", m.Version, m.Name); err != nil {
				rollback()
				runErr = fmt.Errorf("migration %s: record version: %w", m.Path, err)
				return nil
			}
			if err := tx.Commit(ctx); err != nil {
				rollback()
				runErr = fmt.Errorf("migration %s: commit: %w", m.Path, err)
				return nil
			}
			res.Applied = append(res.Applied, m.Path)
			log.InfoContext(ctx, "migration applied", "version", m.Version, "file", m.Path,
				"duration_ms", float64(time.Since(start).Microseconds())/1000)
		}
		if len(res.Applied) == 0 {
			log.InfoContext(ctx, "no new migrations")
		}
		return nil
	})
	if badConn {
		// Lock advisory milik session: buang koneksi agar tidak kembali ke pool.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	if runErr != nil {
		return res, runErr
	}
	if rawErr != nil && !errors.Is(rawErr, driver.ErrBadConn) {
		return res, fmt.Errorf("db: migrate: %w", rawErr)
	}
	return res, nil
}
