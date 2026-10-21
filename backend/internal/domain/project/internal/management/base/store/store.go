// Package store berisi query SQL bersama lintas feature module management (rule 5.3).
// Query yang dipakai satu feature saja tetap di repository.go feature itu.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/db"
)

const projectCols = `p.id::text AS id, p.name, p.description, p.created_by::text AS created_by,
	p.created_by_label, p.created_at, p.updated_at`

const memberCols = `project_id::text AS project_id, user_id::text AS user_id, role,
	added_by::text AS added_by, added_by_label, added_at`

type projectRow struct {
	ID             string    `db:"id"`
	Name           string    `db:"name"`
	Description    string    `db:"description"`
	CreatedBy      *string   `db:"created_by"`
	CreatedByLabel *string   `db:"created_by_label"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

func (r projectRow) toProject() base.Project {
	return base.Project{
		ID: r.ID, Name: r.Name, Description: r.Description,
		CreatedBy: r.CreatedBy, CreatedByLabel: r.CreatedByLabel,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

type accessRow struct {
	projectRow
	MemberRole *string `db:"member_role"`
}

type memberRow struct {
	ProjectID    string    `db:"project_id"`
	UserID       string    `db:"user_id"`
	Role         string    `db:"role"`
	AddedBy      *string   `db:"added_by"`
	AddedByLabel *string   `db:"added_by_label"`
	AddedAt      time.Time `db:"added_at"`
}

func (r memberRow) toMember() base.Member {
	return base.Member{
		ProjectID: r.ProjectID, UserID: r.UserID, Role: r.Role,
		AddedBy: r.AddedBy, AddedByLabel: r.AddedByLabel, AddedAt: r.AddedAt.UTC(),
	}
}

// Store membungkus db.Querier (DB atau transaksi).
type Store struct{ q db.Querier }

func New(q db.Querier) *Store { return &Store{q: q} }

// WithTx mengembalikan Store yang berjalan di dalam tx.
func (s *Store) WithTx(tx *sqlx.Tx) *Store { return &Store{q: tx} }

func validUUID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

// FindProject mengembalikan project. id bukan uuid -> (zero, false, nil) tanpa query.
func (s *Store) FindProject(ctx context.Context, id string) (base.Project, bool, error) {
	return s.getProject(ctx, id, "")
}

// LockProject seperti FindProject dengan FOR UPDATE; wajib dipanggil di dalam tx.
func (s *Store) LockProject(ctx context.Context, id string) (base.Project, bool, error) {
	return s.getProject(ctx, id, " FOR UPDATE")
}

func (s *Store) getProject(ctx context.Context, id, suffix string) (base.Project, bool, error) {
	if !validUUID(id) {
		return base.Project{}, false, nil
	}
	var r projectRow
	err := s.q.GetContext(ctx, &r, `SELECT `+projectCols+` FROM projects p WHERE p.id = $1`+suffix, id)
	if errors.Is(err, sql.ErrNoRows) {
		return base.Project{}, false, nil
	}
	if err != nil {
		return base.Project{}, false, fmt.Errorf("store: find project: %w", err)
	}
	return r.toProject(), true, nil
}

// FindAccess mengembalikan project dan role keanggotaan userID ("" bila bukan anggota).
// userID bukan uuid tetap mencari project dengan memberRole "".
func (s *Store) FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error) {
	if !validUUID(projectID) {
		return base.Project{}, "", false, nil
	}
	var uid any
	if validUUID(userID) {
		uid = userID
	}
	var r accessRow
	err = s.q.GetContext(ctx, &r, `SELECT `+projectCols+`, m.role AS member_role
		FROM projects p
		LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = $2::uuid
		WHERE p.id = $1`, projectID, uid)
	if errors.Is(err, sql.ErrNoRows) {
		return base.Project{}, "", false, nil
	}
	if err != nil {
		return base.Project{}, "", false, fmt.Errorf("store: find access: %w", err)
	}
	if r.MemberRole != nil {
		memberRole = *r.MemberRole
	}
	return r.toProject(), memberRole, true, nil
}

// ListMembers mengembalikan anggota project urut user_id.
func (s *Store) ListMembers(ctx context.Context, projectID string) ([]base.Member, error) {
	return s.members(ctx, projectID, "")
}

// LockMembers seperti ListMembers dengan FOR UPDATE; wajib di dalam tx.
func (s *Store) LockMembers(ctx context.Context, projectID string) ([]base.Member, error) {
	return s.members(ctx, projectID, " FOR UPDATE")
}

func (s *Store) members(ctx context.Context, projectID, suffix string) ([]base.Member, error) {
	if !validUUID(projectID) {
		return []base.Member{}, nil
	}
	var rows []memberRow
	if err := s.q.SelectContext(ctx, &rows,
		`SELECT `+memberCols+` FROM project_members WHERE project_id = $1 ORDER BY user_id`+suffix, projectID); err != nil {
		return nil, fmt.Errorf("store: list members: %w", err)
	}
	out := make([]base.Member, len(rows))
	for i, r := range rows {
		out[i] = r.toMember()
	}
	return out, nil
}
