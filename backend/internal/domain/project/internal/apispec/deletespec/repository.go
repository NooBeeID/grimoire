package deletespec

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/db"
)

type Repo struct {
	db *db.DB
	s  *store.Store
}

func newRepo(d *db.DB, s *store.Store) *Repo { return &Repo{db: d, s: s} }

var _ repository = (*Repo)(nil)

func (r *Repo) InTx(ctx context.Context, fn func(txRepo) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepoImpl{s: r.s.WithTx(tx), q: tx})
	})
}

type txRepoImpl struct {
	s *store.Store
	q db.Querier
}

var _ txRepo = (*txRepoImpl)(nil)

func (t *txRepoImpl) LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error) {
	return t.s.LockSpec(ctx, projectID, name)
}

func (t *txRepoImpl) CountEndpoints(ctx context.Context, specID string) (int, error) {
	return t.s.CountEndpoints(ctx, specID)
}

// DeleteSpec menghapus satu spec; api_spec_endpoints ikut terhapus oleh FK cascade.
func (t *txRepoImpl) DeleteSpec(ctx context.Context, projectID, specID string) (bool, error) {
	if _, err := uuid.Parse(projectID); err != nil {
		return false, nil
	}
	if _, err := uuid.Parse(specID); err != nil {
		return false, nil
	}
	res, err := t.q.ExecContext(ctx, `DELETE FROM api_specs WHERE id = $1 AND project_id = $2`, specID, projectID)
	if err != nil {
		return false, fmt.Errorf("deletespec: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("deletespec: rows affected: %w", err)
	}
	return n == 1, nil
}
