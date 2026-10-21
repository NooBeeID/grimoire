package createkey

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/db"
)

// Repo adalah akses SQL feature createkey.
type Repo struct {
	db *db.DB
	s  *store.Store
}

func newRepo(d *db.DB, s *store.Store) *Repo { return &Repo{db: d, s: s} }

var (
	_ Repository = (*Repo)(nil)
	_ Tx         = (*Repo)(nil)
)

func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&Repo{db: r.db, s: r.s.WithTx(tx)})
	})
}

const (
	qLock  = `SELECT pg_advisory_xact_lock(hashtextextended('apikey:' || $1::text, 0))`
	qCount = `SELECT count(*) FROM api_keys WHERE project_id = $1 AND revoked_at IS NULL`
	qIns   = `INSERT INTO api_keys (project_id, name, prefix, secret_hash, created_by, created_by_label, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING ` + store.KeyColumns
)

func (r *Repo) LockProjectKeys(ctx context.Context, projectID string) error {
	_, err := r.s.Querier().ExecContext(ctx, qLock, projectID)
	return err
}

func (r *Repo) CountActive(ctx context.Context, projectID string) (int, error) {
	var n int
	if err := r.s.Querier().QueryRowxContext(ctx, qCount, projectID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *Repo) Insert(ctx context.Context, k base.Key) (base.Key, error) {
	out, found, err := r.s.GetKey(ctx, qIns, k.ProjectID, k.Name, k.Prefix, k.SecretHash, k.CreatedBy, k.CreatedByLabel, k.CreatedAt)
	if err != nil {
		code, constraint, ok := store.Violation(err)
		switch {
		case ok && code == store.CodeUniqueViolation && constraint == store.ConstraintActiveName:
			return base.Key{}, base.ErrNameTaken
		case ok && code == store.CodeUniqueViolation && constraint == store.ConstraintPrefixUnique:
			return base.Key{}, base.ErrPrefixCollision
		case ok && code == store.CodeFKViolation:
			return base.Key{}, base.ErrProjectNotFound
		}
		return base.Key{}, fmt.Errorf("createkey: insert: %w", err)
	}
	if !found {
		return base.Key{}, fmt.Errorf("createkey: insert: tidak ada baris dikembalikan")
	}
	return out, nil
}
