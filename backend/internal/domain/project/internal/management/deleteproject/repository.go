package deleteproject

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

const deleteProjectSQL = `DELETE FROM projects WHERE id = $1`

// Repo adalah satu-satunya yang menyentuh SQL untuk feature ini.
type Repo struct {
	db *db.DB
	s  *store.Store
}

var _ repository = (*Repo)(nil)

func newRepo(d *db.DB, s *store.Store) *Repo { return &Repo{db: d, s: s} }

func (r *Repo) FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error) {
	return r.s.FindAccess(ctx, projectID, userID)
}

func (r *Repo) InTx(ctx context.Context, fn func(txRepo) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepoImpl{s: r.s.WithTx(tx), q: tx})
	})
}

type txRepoImpl struct {
	s *store.Store
	q sqlx.ExecerContext
}

var _ txRepo = (*txRepoImpl)(nil)

func (t *txRepoImpl) LockProject(ctx context.Context, id string) (base.Project, bool, error) {
	return t.s.LockProject(ctx, id)
}

func (t *txRepoImpl) LockMembers(ctx context.Context, projectID string) ([]base.Member, error) {
	return t.s.LockMembers(ctx, projectID)
}

// DeleteProject: id bukan uuid -> (false, nil) tanpa query. Cascade lewat FK.
func (t *txRepoImpl) DeleteProject(ctx context.Context, id string) (bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return false, nil
	}
	res, err := t.q.ExecContext(ctx, deleteProjectSQL, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
