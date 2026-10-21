package removemember

import (
	"context"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

const deleteMemberSQL = `DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`

// Repo adalah satu-satunya yang menyentuh SQL untuk feature ini.
type Repo struct {
	db *db.DB
	s  *store.Store
}

var _ Repository = (*Repo)(nil)

func newRepo(d *db.DB, s *store.Store) *Repo { return &Repo{db: d, s: s} }

func (r *Repo) FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error) {
	return r.s.FindAccess(ctx, projectID, userID)
}

func (r *Repo) InTx(ctx context.Context, fn func(TxRepository) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
	})
}

type txRepo struct {
	s *store.Store
	q sqlx.ExecerContext
}

var _ TxRepository = (*txRepo)(nil)

func (t *txRepo) LockProject(ctx context.Context, id string) (base.Project, bool, error) {
	return t.s.LockProject(ctx, id)
}

func (t *txRepo) LockMembers(ctx context.Context, projectID string) ([]base.Member, error) {
	return t.s.LockMembers(ctx, projectID)
}

// DeleteMember menghapus satu baris; 0 baris -> ErrMemberNotFound (jaring pengaman).
func (t *txRepo) DeleteMember(ctx context.Context, projectID, userID string) error {
	res, err := t.q.ExecContext(ctx, deleteMemberSQL, projectID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return base.ErrMemberNotFound
	}
	return nil
}
