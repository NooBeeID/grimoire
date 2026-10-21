package countspecs

import (
	"context"

	"grimoire/internal/infra/db"
)

type repo struct{ q db.Querier }

func newRepo(q db.Querier) *repo { return &repo{q: q} }

type countRow struct {
	ProjectID string `db:"project_id"`
	Total     int    `db:"total"`
}

// CountByProjects: ids sudah uuid kanonik, unik, tidak kosong (dijamin usecase).
func (r *repo) CountByProjects(ctx context.Context, ids []string) (map[string]int, error) {
	const q = `SELECT project_id::text AS project_id, count(*)::int AS total
FROM api_specs
WHERE project_id = ANY($1::text[]::uuid[])
GROUP BY project_id`
	var rows []countRow
	if err := r.q.SelectContext(ctx, &rows, q, ids); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.ProjectID] = row.Total
	}
	return out, nil
}
