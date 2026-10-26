package usage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository adalah satu-satunya yang menyentuh SQL di feature ini. Hanya SELECT pada audit_log.
type Repository interface {
	TopFiles(ctx context.Context, w base.Window, projectID string, limit int) ([]base.FileRead, error)
	UnreadPaths(ctx context.Context, w base.Window, projectID string, paths []string, limit int) ([]string, error)
	// ZeroQueries: projectID "" = semua project.
	ZeroQueries(ctx context.Context, w base.Window, projectID string, limit int) ([]base.ZeroQuery, error)
	// PerKey: projectID "" = semua project.
	PerKey(ctx context.Context, w base.Window, projectID string, limit int) ([]base.KeyUsage, error)
}

type sqlRepository struct{ d *db.DB }

// NewRepository tidak mendereferensi d.
func NewRepository(d *db.DB) Repository { return &sqlRepository{d: d} }

// normQuery meniru NormalizeQuery (T11): lower + spasi tunggal + trim.
const normQuery = `lower(btrim(regexp_replace(args->>'query', '[[:space:]]+', ' ', 'g')))`

type topFileRow struct {
	Path       string    `db:"path"`
	Reads      int       `db:"reads"`
	Keys       int       `db:"keys"`
	LastReadAt time.Time `db:"last_read_at"`
}

func (r *sqlRepository) TopFiles(ctx context.Context, w base.Window, projectID string, limit int) ([]base.FileRead, error) {
	const q = `SELECT target AS path,
       count(*) AS reads,
       count(DISTINCT actor_id) AS keys,
       max(occurred_at) AS last_read_at
FROM audit_log
WHERE action = ANY($1::text[]) AND status = 'ok' AND project_id = $2::uuid AND target <> ''
  AND occurred_at >= $3 AND occurred_at < $4
GROUP BY target
ORDER BY reads DESC, target COLLATE "C" ASC
LIMIT $5`
	var rows []topFileRow
	if err := r.d.SelectContext(ctx, &rows, q, base.ReadActions(), projectID, w.From, w.To, limit); err != nil {
		return nil, fmt.Errorf("usage: top files: %w", err)
	}
	out := make([]base.FileRead, 0, len(rows))
	for _, x := range rows {
		out = append(out, base.FileRead{Path: x.Path, Reads: x.Reads, Keys: x.Keys, LastReadAt: x.LastReadAt.UTC()})
	}
	return out, nil
}

func (r *sqlRepository) UnreadPaths(ctx context.Context, w base.Window, projectID string, paths []string, limit int) ([]string, error) {
	if len(paths) == 0 {
		return []string{}, nil
	}
	const q = `SELECT p FROM unnest($1::text[]) AS p
WHERE NOT EXISTS (
  SELECT 1 FROM audit_log a
  WHERE a.project_id = $2::uuid AND a.action = ANY($3::text[]) AND a.status = 'ok'
    AND a.occurred_at >= $4 AND a.occurred_at < $5 AND a.target = p
)
ORDER BY p COLLATE "C"
LIMIT $6`
	out := []string{}
	if err := r.d.SelectContext(ctx, &out, q, paths, projectID, base.ReadActions(), w.From, w.To, limit); err != nil {
		return nil, fmt.Errorf("usage: unread paths: %w", err)
	}
	return out, nil
}

type zeroQueryRow struct {
	Query  string    `db:"query"`
	Count  int       `db:"count"`
	Keys   int       `db:"keys"`
	LastAt time.Time `db:"last_at"`
}

func (r *sqlRepository) ZeroQueries(ctx context.Context, w base.Window, projectID string, limit int) ([]base.ZeroQuery, error) {
	// $2 = '' berarti semua project; cast uuid hanya dievaluasi bila terisi.
	q := `SELECT query, count(*) AS count, count(DISTINCT actor_id) AS keys, max(occurred_at) AS last_at
FROM (
  SELECT ` + normQuery + ` AS query, actor_id, occurred_at
  FROM audit_log
  WHERE action = $1 AND status = 'ok'
    AND occurred_at >= $3 AND occurred_at < $4
    AND ($2 = '' OR project_id = NULLIF($2, '')::uuid)
    AND CASE WHEN jsonb_typeof(result->'total') = 'number' THEN (result->>'total')::numeric = 0 ELSE false END
    AND jsonb_typeof(args->'query') = 'string'
) s
WHERE query <> ''
GROUP BY query
ORDER BY count DESC, query COLLATE "C" ASC
LIMIT $5`
	var rows []zeroQueryRow
	if err := r.d.SelectContext(ctx, &rows, q, record.ActionMCPSearchKnowledge, projectID, w.From, w.To, limit); err != nil {
		return nil, fmt.Errorf("usage: zero queries: %w", err)
	}
	out := make([]base.ZeroQuery, 0, len(rows))
	for _, x := range rows {
		out = append(out, base.ZeroQuery{Query: x.Query, Count: x.Count, Keys: x.Keys, LastAt: x.LastAt.UTC()})
	}
	return out, nil
}

type perKeyRow struct {
	KeyID      string         `db:"key_id"`
	Label      string         `db:"label"`
	Calls      int            `db:"calls"`
	Errors     int            `db:"errors"`
	Searches   int            `db:"searches"`
	Reads      int            `db:"reads"`
	LastUsedAt time.Time      `db:"last_used_at"`
	TopTool    sql.NullString `db:"top_tool"`
}

func (r *sqlRepository) PerKey(ctx context.Context, w base.Window, projectID string, limit int) ([]base.KeyUsage, error) {
	const q = `WITH e AS (
  SELECT actor_id, actor_label, action, status, occurred_at, id
  FROM audit_log
  WHERE actor_type = 'api_key' AND actor_id IS NOT NULL AND action = ANY($1::text[])
    AND occurred_at >= $4 AND occurred_at < $5
    AND ($3 = '' OR project_id = NULLIF($3, '')::uuid)
), tools AS (
  SELECT DISTINCT ON (actor_id) actor_id, action
  FROM (SELECT actor_id, action, count(*) AS n FROM e GROUP BY actor_id, action) t
  ORDER BY actor_id, n DESC, action ASC
)
SELECT e.actor_id::text AS key_id,
       (array_agg(e.actor_label ORDER BY e.occurred_at DESC, e.id DESC))[1] AS label,
       count(*) AS calls,
       count(*) FILTER (WHERE e.status = 'error') AS errors,
       count(*) FILTER (WHERE e.action = $2) AS searches,
       count(*) FILTER (WHERE e.action = ANY($6::text[]) AND e.status = 'ok') AS reads,
       max(e.occurred_at) AS last_used_at,
       max(t.action) AS top_tool
FROM e JOIN tools t ON t.actor_id = e.actor_id
GROUP BY e.actor_id
ORDER BY calls DESC, label ASC, key_id ASC
LIMIT $7`
	var rows []perKeyRow
	if err := r.d.SelectContext(ctx, &rows, q, base.ToolActions(), record.ActionMCPSearchKnowledge, projectID, w.From, w.To, base.ReadActions(), limit); err != nil {
		return nil, fmt.Errorf("usage: per key: %w", err)
	}
	out := make([]base.KeyUsage, 0, len(rows))
	for _, x := range rows {
		out = append(out, base.KeyUsage{
			KeyID: x.KeyID, Label: x.Label, Calls: x.Calls, Errors: x.Errors, Searches: x.Searches,
			Reads: x.Reads, LastUsedAt: x.LastUsedAt.UTC(), TopTool: x.TopTool.String,
		})
	}
	return out, nil
}
