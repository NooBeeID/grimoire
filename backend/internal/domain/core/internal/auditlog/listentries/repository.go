package listentries

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/db"
)

// Repository adalah satu-satunya yang menyentuh SQL di feature ini.
type Repository interface {
	// List mengembalikan paling banyak limit baris (kolom summary saja), urut
	// occurred_at DESC, id DESC, setelah f.Cursor bila ada.
	List(ctx context.Context, f base.Filter, limit int) ([]base.Entry, error)
}

type sqlRepository struct{ q db.Querier }

// NewRepository membuat Repository berbasis SQL.
func NewRepository(q db.Querier) Repository { return &sqlRepository{q: q} }

const selectSQL = `SELECT id::text, occurred_at, actor_type, actor_id::text, actor_label, ` +
	`project_id::text, project_label, action, target_type, target, status FROM audit_log`

// buildQuery merakit SQL dari fragmen literal; nilai selalu lewat placeholder.
func buildQuery(f base.Filter, limit int) (string, []any) {
	var args []any
	add := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	var preds []string
	if f.ActorType != "" {
		preds = append(preds, "actor_type = "+add(f.ActorType))
	}
	if f.ActorID != "" {
		preds = append(preds, "actor_id = "+add(f.ActorID)+"::uuid")
	}
	if f.ActorLabel != "" {
		preds = append(preds, "actor_label ILIKE "+add("%"+base.EscapeLike(f.ActorLabel)+"%")+` ESCAPE '\'`)
	}
	if f.ProjectID != "" {
		preds = append(preds, "project_id = "+add(f.ProjectID)+"::uuid")
	}
	if len(f.Actions) > 0 {
		preds = append(preds, "action = ANY("+add(f.Actions)+"::text[])")
	}
	if len(f.Statuses) > 0 {
		preds = append(preds, "status = ANY("+add(f.Statuses)+"::text[])")
	}
	if !f.From.IsZero() {
		preds = append(preds, "occurred_at >= "+add(f.From))
	}
	if !f.To.IsZero() {
		preds = append(preds, "occurred_at < "+add(f.To))
	}
	if f.Target != "" {
		preds = append(preds, "target ILIKE "+add("%"+base.EscapeLike(f.Target)+"%")+` ESCAPE '\'`)
	}
	if f.Cursor != nil {
		t := add(f.Cursor.OccurredAt)
		id := add(f.Cursor.ID)
		preds = append(preds, "(occurred_at, id) < ("+t+"::timestamptz, "+id+"::uuid)")
	}
	var sb strings.Builder
	sb.WriteString(selectSQL)
	if len(preds) > 0 {
		sb.WriteString(" WHERE " + strings.Join(preds, " AND "))
	}
	sb.WriteString(" ORDER BY occurred_at DESC, id DESC LIMIT " + add(limit))
	return sb.String(), args
}

type row struct {
	ID           string         `db:"id"`
	OccurredAt   time.Time      `db:"occurred_at"`
	ActorType    string         `db:"actor_type"`
	ActorID      sql.NullString `db:"actor_id"`
	ActorLabel   string         `db:"actor_label"`
	ProjectID    sql.NullString `db:"project_id"`
	ProjectLabel sql.NullString `db:"project_label"`
	Action       string         `db:"action"`
	TargetType   sql.NullString `db:"target_type"`
	Target       sql.NullString `db:"target"`
	Status       string         `db:"status"`
}

func (r *sqlRepository) List(ctx context.Context, f base.Filter, limit int) ([]base.Entry, error) {
	query, args := buildQuery(f, limit)
	var rows []row
	if err := r.q.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("listentries: query: %w", err)
	}
	out := make([]base.Entry, 0, len(rows))
	for _, rw := range rows {
		out = append(out, base.Entry{
			ID:           rw.ID,
			OccurredAt:   rw.OccurredAt.UTC(),
			ActorType:    rw.ActorType,
			ActorID:      rw.ActorID.String,
			ActorLabel:   rw.ActorLabel,
			ProjectID:    rw.ProjectID.String,
			ProjectLabel: rw.ProjectLabel.String,
			Action:       rw.Action,
			TargetType:   rw.TargetType.String,
			Target:       rw.Target.String,
			Status:       rw.Status,
		})
	}
	return out, nil
}
