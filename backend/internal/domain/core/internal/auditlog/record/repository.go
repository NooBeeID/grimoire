package record

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

const insertSQL = `INSERT INTO audit_log
  (id, occurred_at, actor_type, actor_id, actor_label, project_id, project_label,
   action, target_type, target, args, status, result, ip, user_agent, trace_id)
VALUES
  ($1::uuid, $2, $3, $4::uuid, $5, $6::uuid, $7, $8, $9, $10, $11::jsonb, $12, $13::jsonb, $14::inet, $15, $16)`

// Repo menulis entri audit ke Postgres.
type Repo struct{ db sqlx.ExtContext }

// NewRepo membuat Repo.
func NewRepo(db sqlx.ExtContext) *Repo { return &Repo{db: db} }

// Insert menyimpan satu entri dengan satu INSERT parameterized.
func (r *Repo) Insert(ctx context.Context, e base.Entry) error {
	args, err := json.Marshal(nonNil(e.Args))
	if err != nil {
		return err
	}
	result, err := json.Marshal(nonNil(e.Result))
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, insertSQL,
		e.ID, e.OccurredAt, e.ActorType, nullIfEmpty(e.ActorID), e.ActorLabel,
		nullIfEmpty(e.ProjectID), nullIfEmpty(e.ProjectLabel), e.Action,
		nullIfEmpty(e.TargetType), nullIfEmpty(e.Target), string(args), e.Status,
		string(result), nullIfEmpty(e.IP), nullIfEmpty(e.UserAgent), e.TraceID,
	)
	return err
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
