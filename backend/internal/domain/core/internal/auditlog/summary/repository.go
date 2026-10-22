package summary

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

// Repository adalah satu-satunya yang menyentuh SQL di feature ini. Hanya SELECT.
type Repository interface {
	// Load menjalankan tiga query agregat dalam satu tx ReadOnly REPEATABLE READ.
	// projectID "" = semua project.
	Load(ctx context.Context, w base.Window, projectID string) (base.Aggregates, error)
}

type sqlRepository struct{ d *db.DB }

// NewRepository tidak mendereferensi d.
func NewRepository(d *db.DB) Repository { return &sqlRepository{d: d} }

// QualityReporter adalah port ke provider discovery.knowledgetools.search-quality-report.
type QualityReporter interface {
	Report(ctx context.Context, req searchquality.Request) (searchquality.Report, error)
}

type busQualityReporter struct{ b *bus.Registry }

// NewQualityReporter membuat adapter bus.
func NewQualityReporter(b *bus.Registry) QualityReporter { return &busQualityReporter{b: b} }

func (r *busQualityReporter) Report(ctx context.Context, req searchquality.Request) (searchquality.Report, error) {
	res, err := bus.CallTyped[searchquality.Response](ctx, r.b, constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, req)
	return res.Report, err
}

// buildWhere membentuk predikat bersama; nilai lewat placeholder.
func buildWhere(w base.Window, projectID string) (string, []any) {
	where := "action = ANY($1::text[]) AND occurred_at >= $2 AND occurred_at < $3"
	args := []any{base.ToolActions(), w.From, w.To}
	if projectID != "" {
		where += " AND project_id = $4::uuid"
		args = append(args, projectID)
	}
	return where, args
}

type toolRow struct {
	Action string          `db:"action"`
	Calls  int             `db:"calls"`
	OK     int             `db:"ok"`
	Errors int             `db:"errors"`
	P50    sql.NullFloat64 `db:"p50_ms"`
	P95    sql.NullFloat64 `db:"p95_ms"`
}

type projectRow struct {
	ProjectID    string `db:"project_id"`
	ProjectLabel string `db:"project_label"`
	Calls        int    `db:"calls"`
}

type trafficRow struct {
	At     time.Time `db:"at"`
	Calls  int       `db:"calls"`
	Errors int       `db:"errors"`
}

const durationExpr = `CASE WHEN jsonb_typeof(result->'duration_ms') = 'number' THEN (result->>'duration_ms')::numeric END`

func (r *sqlRepository) Load(ctx context.Context, w base.Window, projectID string) (base.Aggregates, error) {
	tx, err := r.d.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return base.Aggregates{}, fmt.Errorf("summary: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	where, args := buildWhere(w, projectID)

	var tools []toolRow
	q1 := `SELECT action,
       count(*) AS calls,
       count(*) FILTER (WHERE status = 'ok') AS ok,
       count(*) FILTER (WHERE status = 'error') AS errors,
       percentile_cont(0.5) WITHIN GROUP (ORDER BY ` + durationExpr + `) AS p50_ms,
       percentile_cont(0.95) WITHIN GROUP (ORDER BY ` + durationExpr + `) AS p95_ms
FROM audit_log
WHERE ` + where + `
GROUP BY action`
	if err := tx.SelectContext(ctx, &tools, q1, args...); err != nil {
		return base.Aggregates{}, fmt.Errorf("summary: tools: %w", err)
	}

	var projects []projectRow
	n := len(args)
	q2 := fmt.Sprintf(`SELECT project_id::text AS project_id,
       COALESCE((array_agg(project_label ORDER BY occurred_at DESC, id DESC))[1], '') AS project_label,
       count(*) AS calls
FROM audit_log
WHERE %s AND project_id IS NOT NULL
GROUP BY project_id
ORDER BY calls DESC, project_id ASC
LIMIT $%d`, where, n+1)
	if err := tx.SelectContext(ctx, &projects, q2, append(append([]any{}, args...), base.TopProjectsLimit)...); err != nil {
		return base.Aggregates{}, fmt.Errorf("summary: projects: %w", err)
	}

	var traffic []trafficRow
	unit := string(w.Bucket) // whitelist hour|day, bukan input pengguna
	q3 := fmt.Sprintf(`WITH t AS (
  SELECT date_trunc('%[1]s', occurred_at, 'UTC') AS at,
         count(*) AS calls,
         count(*) FILTER (WHERE status = 'error') AS errors
  FROM audit_log
  WHERE %[2]s
  GROUP BY 1
)
SELECT b.at AS at, COALESCE(t.calls, 0) AS calls, COALESCE(t.errors, 0) AS errors
FROM generate_series(date_trunc('%[1]s', $2::timestamptz, 'UTC'),
                     $3::timestamptz - interval '1 microsecond',
                     '%[3]s'::interval) AS b(at)
LEFT JOIN t ON t.at = b.at
ORDER BY b.at`, unit, where, w.Bucket.Interval())
	if err := tx.SelectContext(ctx, &traffic, q3, args...); err != nil {
		return base.Aggregates{}, fmt.Errorf("summary: traffic: %w", err)
	}

	agg := base.Aggregates{
		Tools:    make([]base.ToolAgg, 0, len(tools)),
		Projects: make([]base.ProjectAgg, 0, len(projects)),
		Traffic:  make([]base.TrafficPoint, 0, len(traffic)),
	}
	for _, t := range tools {
		a := base.ToolAgg{Action: t.Action, Calls: t.Calls, OK: t.OK, Errors: t.Errors}
		if t.P50.Valid {
			v := t.P50.Float64
			a.P50Ms = &v
		}
		if t.P95.Valid {
			v := t.P95.Float64
			a.P95Ms = &v
		}
		agg.Tools = append(agg.Tools, a)
	}
	for _, p := range projects {
		agg.Projects = append(agg.Projects, base.ProjectAgg{ProjectID: p.ProjectID, ProjectLabel: p.ProjectLabel, Calls: p.Calls})
	}
	for _, p := range traffic {
		agg.Traffic = append(agg.Traffic, base.TrafficPoint{At: p.At.UTC(), Calls: p.Calls, Errors: p.Errors})
	}
	return agg, nil
}
