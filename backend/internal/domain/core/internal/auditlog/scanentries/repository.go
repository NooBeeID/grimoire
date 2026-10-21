package scanentries

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/core/audit/scanentries"
)

// Repository adalah satu-satunya yang menyentuh SQL di feature ini.
type Repository interface {
	// Scan mengembalikan paling banyak limit baris project p.ProjectID, urut occurred_at ASC, id ASC,
	// setelah p.After bila ada.
	Scan(ctx context.Context, p params, limit int) ([]scanentries.Entry, error)
}

type sqlRepository struct{ q db.Querier }

// NewRepository membuat Repository berbasis SQL.
func NewRepository(q db.Querier) Repository { return &sqlRepository{q: q} }

const selectSQL = `SELECT id::text, occurred_at, actor_type, actor_id::text, action, target, status, args, result FROM audit_log`

// buildQuery merakit SQL dari fragmen literal; nilai selalu lewat placeholder.
func buildQuery(p params, limit int) (string, []any) {
	var args []any
	add := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	preds := []string{
		"project_id = " + add(p.ProjectID) + "::uuid",
		"action = ANY(" + add(p.Actions) + "::text[])",
		"occurred_at >= " + add(p.From),
		"occurred_at < " + add(p.To),
	}
	if p.After != nil {
		t := add(p.After.OccurredAt)
		id := add(p.After.ID)
		preds = append(preds, "(occurred_at, id) > ("+t+"::timestamptz, "+id+"::uuid)")
	}
	var sb strings.Builder
	sb.WriteString(selectSQL)
	sb.WriteString(" WHERE " + strings.Join(preds, " AND "))
	sb.WriteString(" ORDER BY occurred_at ASC, id ASC LIMIT " + add(limit))
	return sb.String(), args
}

type row struct {
	ID         string         `db:"id"`
	OccurredAt time.Time      `db:"occurred_at"`
	ActorType  string         `db:"actor_type"`
	ActorID    sql.NullString `db:"actor_id"`
	Action     string         `db:"action"`
	Target     sql.NullString `db:"target"`
	Status     string         `db:"status"`
	Args       []byte         `db:"args"`
	Result     []byte         `db:"result"`
}

// decodeObject mengubah jsonb menjadi map; kosong/null -> map kosong non-nil.
func decodeObject(raw []byte) (map[string]any, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func (r *sqlRepository) Scan(ctx context.Context, p params, limit int) ([]scanentries.Entry, error) {
	query, args := buildQuery(p, limit)
	var rows []row
	if err := r.q.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("scanentries: query: %w", err)
	}
	out := make([]scanentries.Entry, 0, len(rows))
	for _, rw := range rows {
		a, err := decodeObject(rw.Args)
		if err != nil {
			return nil, fmt.Errorf("scanentries: decode args: %w", err)
		}
		res, err := decodeObject(rw.Result)
		if err != nil {
			return nil, fmt.Errorf("scanentries: decode result: %w", err)
		}
		out = append(out, scanentries.Entry{
			ID:         rw.ID,
			OccurredAt: rw.OccurredAt.UTC(),
			ActorType:  rw.ActorType,
			ActorID:    rw.ActorID.String,
			Action:     rw.Action,
			Target:     rw.Target.String,
			Status:     rw.Status,
			Args:       a,
			Result:     res,
		})
	}
	return out, nil
}
