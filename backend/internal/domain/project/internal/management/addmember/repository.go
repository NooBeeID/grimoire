package addmember

import (
	"context"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

const insertMemberSQL = `INSERT INTO project_members (project_id, user_id, role, added_by, added_by_label, added_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (project_id, user_id) DO NOTHING`

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

func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
	})
}

type txRepo struct {
	s *store.Store
	q sqlx.ExecerContext
}

var _ Tx = (*txRepo)(nil)

func (t *txRepo) LockProject(ctx context.Context, id string) (base.Project, bool, error) {
	return t.s.LockProject(ctx, id)
}

func (t *txRepo) LockMembers(ctx context.Context, projectID string) ([]base.Member, error) {
	return t.s.LockMembers(ctx, projectID)
}

func (t *txRepo) InsertMember(ctx context.Context, m base.Member) (bool, error) {
	res, err := t.q.ExecContext(ctx, insertMemberSQL,
		m.ProjectID, m.UserID, m.Role, m.AddedBy, m.AddedByLabel, m.AddedAt.UTC())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
