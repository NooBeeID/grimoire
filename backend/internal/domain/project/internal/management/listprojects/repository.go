package listprojects

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/db"
)

const (
	joinMember     = "JOIN"
	leftJoinMember = "LEFT JOIN"

	selectListSQL = `SELECT p.id::text AS id, p.name, p.description,
       p.created_by::text AS created_by, p.created_by_label,
       p.created_at, p.updated_at,
       m.role AS my_role,
       (SELECT count(*) FROM project_members pm WHERE pm.project_id = p.id) AS member_count,
       COUNT(*) OVER() AS total
FROM projects p `
	selectCountSQL = `SELECT count(*) FROM projects p `
)

type Item struct {
	Project     base.Project
	MyRole      string // "" bila aktor bukan anggota (hanya mungkin untuk super_admin)
	MemberCount int
}

type Repository interface {
	// List mengembalikan satu halaman urut updated_at DESC, id DESC dan total baris cocok filter.
	// restrictToMember=true (developer): JOIN keanggotaan aktor. false (super_admin): LEFT JOIN.
	List(ctx context.Context, actorID string, restrictToMember bool, f ProjectFilter) (items []Item, total int, err error)
}

type Repo struct{ q db.Querier }

var _ Repository = (*Repo)(nil)

func NewRepo(q db.Querier) *Repo { return &Repo{q: q} }

type projectRow struct {
	ID             string    `db:"id"`
	Name           string    `db:"name"`
	Description    string    `db:"description"`
	CreatedBy      *string   `db:"created_by"`
	CreatedByLabel *string   `db:"created_by_label"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
	MyRole         *string   `db:"my_role"`
	MemberCount    int       `db:"member_count"`
	Total          int       `db:"total"`
}

// build menghasilkan klausa JOIN + WHERE dan argumen ($1 = aktor) yang dipakai kedua query.
func build(f ProjectFilter, actorID string, restrict bool) (clause string, args []any) {
	aid := uuid.Nil
	if id, err := uuid.Parse(actorID); err == nil {
		aid = id
	}
	args = []any{aid.String()}
	j := leftJoinMember
	if restrict {
		j = joinMember
	}
	var sb strings.Builder
	sb.WriteString(j + " project_members m ON m.project_id = p.id AND m.user_id = $1::uuid\nWHERE 1=1")
	if pat := f.LikePattern(); pat != "" {
		args = append(args, pat)
		sb.WriteString(` AND p.name ILIKE $` + strconv.Itoa(len(args)) + ` ESCAPE '\'`)
	}
	return sb.String(), args
}

func (r *Repo) List(ctx context.Context, actorID string, restrict bool, f ProjectFilter) ([]Item, int, error) {
	clause, args := build(f, actorID, restrict)
	n := len(args)
	query := selectListSQL + clause +
		"\nORDER BY p.updated_at DESC, p.id DESC\nLIMIT $" + strconv.Itoa(n+1) + " OFFSET $" + strconv.Itoa(n+2)
	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)

	var rows []projectRow
	if err := r.q.SelectContext(ctx, &rows, query, listArgs...); err != nil {
		return nil, 0, fmt.Errorf("listprojects: query projects: %w", err)
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		it := Item{
			Project: base.Project{
				ID: row.ID, Name: row.Name, Description: row.Description,
				CreatedBy: row.CreatedBy, CreatedByLabel: row.CreatedByLabel,
				CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
			},
			MemberCount: row.MemberCount,
		}
		if row.MyRole != nil {
			it.MyRole = *row.MyRole
		}
		items = append(items, it)
	}
	switch {
	case len(rows) > 0:
		return items, rows[0].Total, nil
	case f.Offset > 0:
		var total int
		if err := r.q.GetContext(ctx, &total, selectCountSQL+clause, args...); err != nil {
			return nil, 0, fmt.Errorf("listprojects: count projects: %w", err)
		}
		return items, total, nil
	}
	return items, 0, nil
}
