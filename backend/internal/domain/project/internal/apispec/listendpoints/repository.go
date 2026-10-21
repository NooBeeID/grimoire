package listendpoints

import (
	"context"
	"encoding/json"
	"fmt"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/db"
)

// Row = satu baris hasil: nama spec + endpoint (Tags tidak nil).
type Row struct {
	Spec string
	base.Endpoint
}

type Repo struct {
	q db.Querier
	s *store.Store
}

func newRepo(q db.Querier, s *store.Store) *Repo { return &Repo{q: q, s: s} }

const whereClause = `
WHERE s.project_id = $1::uuid
  AND ($2::text = '' OR s.name = $2)
  AND ($3::text = '' OR $3 = ANY(e.tags))
  AND ($4::text = '' OR e.path ILIKE $4 ESCAPE '\'
                     OR e.summary ILIKE $4 ESCAPE '\'
                     OR e.operation_id ILIKE $4 ESCAPE '\')`

const mainQuery = `SELECT s.name AS spec, e.method, e.path, e.operation_id, e.summary,
       to_json(e.tags)::text AS tags, e.deprecated, COUNT(*) OVER() AS total
FROM api_specs s
JOIN api_spec_endpoints e ON e.spec_id = s.id` + whereClause + `
ORDER BY s.name COLLATE "C", e.path COLLATE "C",
         array_position(ARRAY['GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE'], e.method)
LIMIT $5 OFFSET $6`

const countQuery = `SELECT count(*) FROM api_specs s JOIN api_spec_endpoints e ON e.spec_id = s.id` + whereClause

type queryRow struct {
	Spec        string  `db:"spec"`
	Method      string  `db:"method"`
	Path        string  `db:"path"`
	OperationID *string `db:"operation_id"`
	Summary     string  `db:"summary"`
	Tags        string  `db:"tags"`
	Deprecated  bool    `db:"deprecated"`
	Total       int     `db:"total"`
}

// ListSpecNames mendelegasikan ke store.ListSpecNames.
func (r *Repo) ListSpecNames(ctx context.Context, projectID string) ([]string, error) {
	return r.s.ListSpecNames(ctx, projectID)
}

// QueryEndpoints menjalankan query utama; mengembalikan baris halaman ini dan total.
func (r *Repo) QueryEndpoints(ctx context.Context, f Filter) ([]Row, int, error) {
	var raw []queryRow
	if err := r.q.SelectContext(ctx, &raw, mainQuery, f.ProjectID, f.Spec, f.Tag, f.Pattern, f.Limit, f.Offset); err != nil {
		return nil, 0, fmt.Errorf("listendpoints: query endpoints: %w", err)
	}
	rows := make([]Row, 0, len(raw))
	total := 0
	for i, x := range raw {
		if i == 0 {
			total = x.Total
		}
		tags := []string{}
		if err := json.Unmarshal([]byte(x.Tags), &tags); err != nil {
			return nil, 0, fmt.Errorf("listendpoints: decode tags: %w", err)
		}
		if tags == nil {
			tags = []string{}
		}
		rows = append(rows, Row{Spec: x.Spec, Endpoint: base.Endpoint{
			Method: x.Method, Path: x.Path, OperationID: x.OperationID,
			Summary: x.Summary, Tags: tags, Deprecated: x.Deprecated,
		}})
	}
	if len(rows) == 0 && f.Offset > 0 {
		if err := r.q.GetContext(ctx, &total, countQuery, f.ProjectID, f.Spec, f.Tag, f.Pattern); err != nil {
			return nil, 0, fmt.Errorf("listendpoints: count endpoints: %w", err)
		}
	}
	return rows, total, nil
}
