package runsync

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/base/store"
	"grimoire/internal/infra/db"
)

// Repository adalah satu-satunya akses SQL usecase ini.
type Repository interface {
	// TryLock: ok=false, err=nil bila lock dipegang sync lain. unlock dipanggil tepat sekali bila ok=true.
	TryLock(ctx context.Context, projectID string, kind base.Kind) (unlock func(context.Context) error, ok bool, err error)
	UpsertConfig(ctx context.Context, c base.Config) (base.Config, error)
	SaveResult(ctx context.Context, p SaveResultParams) (base.Config, error)
}

// SaveResultParams adalah hasil sync yang disimpan ke config.
type SaveResultParams struct {
	ProjectID     string
	Kind          base.Kind
	Status        string // success|partial|failed
	SyncedAt      time.Time
	ByID, ByLabel string
	Result        base.Report
}

// Repo mengimplementasi Repository di atas Postgres.
type Repo struct {
	db *db.DB
	s  *store.Store
}

var _ Repository = (*Repo)(nil)

func newRepo(d *db.DB, s *store.Store) *Repo { return &Repo{db: d, s: s} }

// TryLock mengambil advisory lock sesi pada satu koneksi yang dipin sampai unlock.
func (r *Repo) TryLock(ctx context.Context, projectID string, kind base.Kind) (func(context.Context) error, bool, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := r.db.Connx(cctx)
	if err != nil {
		return nil, false, err
	}
	var got bool
	if err := conn.GetContext(cctx, &got, "SELECT pg_try_advisory_lock("+store.LockKeySQL+")", projectID, string(kind)); err != nil {
		discard(conn)
		return nil, false, err
	}
	if !got {
		_ = conn.Close()
		return nil, false, nil
	}
	return func(uctx context.Context) error {
		var released bool
		err := conn.GetContext(uctx, &released, "SELECT pg_advisory_unlock("+store.LockKeySQL+")", projectID, string(kind))
		if err != nil || !released {
			discard(conn)
			if err == nil {
				err = errors.New("runsync: advisory unlock returned false")
			}
			return err
		}
		return conn.Close()
	}, true, nil
}

// discard membuang koneksi dari pool; menutup koneksi fisik melepas lock sesi.
func discard(conn *sqlx.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

// UpsertConfig menyimpan asal repo; kolom last_* tidak disentuh.
func (r *Repo) UpsertConfig(ctx context.Context, c base.Config) (base.Config, error) {
	var row store.Row
	err := r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return tx.GetContext(ctx, &row, `INSERT INTO git_sync_configs
 (project_id, kind, provider, host, repo_url, repo_path, branch, folder, target_prefix)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
 ON CONFLICT (project_id, kind) DO UPDATE SET
   provider=EXCLUDED.provider, host=EXCLUDED.host, repo_url=EXCLUDED.repo_url, repo_path=EXCLUDED.repo_path,
   branch=EXCLUDED.branch, folder=EXCLUDED.folder, target_prefix=EXCLUDED.target_prefix, updated_at=now()
 RETURNING `+store.ConfigColumns,
			c.ProjectID, string(c.Kind), c.Provider, c.Host, c.RepoURL, c.RepoPath, c.Branch, c.Folder, c.TargetPrefix)
	})
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" {
			return base.Config{}, base.ErrProjectNotFound
		}
		return base.Config{}, fmt.Errorf("runsync: upsert config: %w", err)
	}
	return store.ScanConfig(row), nil
}

// SaveResult mencatat hasil sync terakhir.
func (r *Repo) SaveResult(ctx context.Context, p SaveResultParams) (base.Config, error) {
	js, err := store.MarshalResult(p.Result)
	if err != nil {
		return base.Config{}, fmt.Errorf("runsync: marshal result: %w", err)
	}
	var byID any
	if p.ByID != "" {
		byID = p.ByID
	}
	var row store.Row
	err = r.db.GetContext(ctx, &row, `UPDATE git_sync_configs SET last_status=$3, last_synced_at=$4,
 last_synced_by=$5::uuid, last_synced_by_label=$6, last_result=$7::jsonb, updated_at=now()
 WHERE project_id=$1 AND kind=$2 RETURNING `+store.ConfigColumns,
		p.ProjectID, string(p.Kind), p.Status, p.SyncedAt, byID, p.ByLabel, string(js))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.Config{}, base.ErrProjectNotFound
		}
		return base.Config{}, fmt.Errorf("runsync: save result: %w", err)
	}
	return store.ScanConfig(row), nil
}
