package createproject

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/db"
)

const (
	insertProjectSQL = `
INSERT INTO projects (name, description, created_by, created_by_label, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING id::text, created_at, updated_at`

	insertMemberSQL = `
INSERT INTO project_members (project_id, user_id, role, added_by, added_by_label, added_at)
VALUES ($1, $2, $3, $4, $5, $6)`
)

// Repo adalah satu-satunya yang menyentuh SQL untuk feature ini.
type Repo struct {
	db *db.DB
	q  db.Querier // *db.DB di luar tx, *sqlx.Tx di dalam tx
}

var (
	_ Repository = (*Repo)(nil)
	_ Tx         = (*Repo)(nil)
)

func newRepo(d *db.DB) *Repo { return &Repo{db: d, q: d} }

func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&Repo{db: r.db, q: tx})
	})
}

type insertedRow struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (r *Repo) InsertProject(ctx context.Context, p base.Project) (base.Project, error) {
	var row insertedRow
	if err := r.q.GetContext(ctx, &row, insertProjectSQL,
		p.Name, p.Description, p.CreatedBy, p.CreatedByLabel, p.CreatedAt); err != nil {
		return base.Project{}, err
	}
	p.ID, p.CreatedAt, p.UpdatedAt = row.ID, row.CreatedAt.UTC(), row.UpdatedAt.UTC()
	return p, nil
}

func (r *Repo) InsertMember(ctx context.Context, m base.Member) error {
	_, err := r.q.ExecContext(ctx, insertMemberSQL,
		m.ProjectID, m.UserID, m.Role, m.AddedBy, m.AddedByLabel, m.AddedAt)
	return err
}
