package getentry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/db"
)

const getSQL = `
SELECT id::text, occurred_at, actor_type, actor_id::text, actor_label,
       project_id::text, project_label, action, target_type, target,
       args, result, status, ip::text, user_agent, trace_id
FROM audit_log
WHERE id = $1`

type repository struct{ db db.Querier }

// NewRepository membuat Repository berbasis SQL.
func NewRepository(q db.Querier) Repository { return &repository{db: q} }

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
	Args         []byte         `db:"args"`
	Result       []byte         `db:"result"`
	Status       string         `db:"status"`
	IP           sql.NullString `db:"ip"`
	UserAgent    sql.NullString `db:"user_agent"`
	TraceID      string         `db:"trace_id"`
}

func (r *repository) Get(ctx context.Context, id string) (base.Entry, bool, error) {
	var rw row
	if err := r.db.GetContext(ctx, &rw, getSQL, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.Entry{}, false, nil
		}
		return base.Entry{}, false, fmt.Errorf("select audit_log: %w", err)
	}
	args, err := decodeJSON(rw.Args)
	if err != nil {
		return base.Entry{}, false, fmt.Errorf("decode args: %w", err)
	}
	result, err := decodeJSON(rw.Result)
	if err != nil {
		return base.Entry{}, false, fmt.Errorf("decode result: %w", err)
	}
	return base.Entry{
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
		Args:         args,
		Result:       result,
		Status:       rw.Status,
		IP:           trimHostMask(rw.IP.String),
		UserAgent:    rw.UserAgent.String,
		TraceID:      rw.TraceID,
	}, true, nil
}

func decodeJSON(b []byte) (map[string]any, error) {
	var m map[string]any
	if len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// trimHostMask membuang suffix /32 atau /128 dari representasi inet host.
func trimHostMask(s string) string {
	if strings.Contains(s, ".") {
		return strings.TrimSuffix(s, "/32")
	}
	return strings.TrimSuffix(s, "/128")
}
