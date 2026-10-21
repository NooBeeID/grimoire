package updateproject

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

const updateProjectSQL = `UPDATE projects SET name = $2, description = $3, updated_at = $4 WHERE id = $1 RETURNING updated_at`

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

func (r *Repo) ListMembers(ctx context.Context, projectID string) ([]base.Member, error) {
	return r.s.ListMembers(ctx, projectID)
}

func (r *Repo) InTx(ctx context.Context, fn func(TxRepository) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
	})
}

type txRepo struct {
	s *store.Store
	q sqlx.ExtContext
}

var _ TxRepository = (*txRepo)(nil)

func (t *txRepo) LockProject(ctx context.Context, id string) (base.Project, bool, error) {
	return t.s.LockProject(ctx, id)
}

func (t *txRepo) UpdateProject(ctx context.Context, p base.Project) (time.Time, error) {
	var at time.Time
	err := sqlx.GetContext(ctx, t.q, &at, updateProjectSQL, p.ID, p.Name, p.Description, p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, base.ErrProjectNotFound
	}
	if err != nil {
		return time.Time{}, err
	}
	return at.UTC(), nil
}
