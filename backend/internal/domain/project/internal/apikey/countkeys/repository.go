package countkeys

import (
	"context"

	"grimoire/internal/domain/project/internal/apikey/base/store"
)

// Repo adalah akses SQL untuk feature countkeys.
type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

type countRow struct {
	ProjectID string `db:"project_id"`
	N         int    `db:"n"`
}

const qCountActive = `
SELECT project_id::text AS project_id, count(*) AS n
FROM api_keys
WHERE project_id = ANY($1::text[]::uuid[]) AND revoked_at IS NULL
GROUP BY project_id`

// CountActiveByProjects: ids sudah kanonik, unik, 1..100 (dijamin usecase). Hanya key aktif.
func (r *Repo) CountActiveByProjects(ctx context.Context, ids []string) (map[string]int, error) {
	var rows []countRow
	if err := r.s.Querier().SelectContext(ctx, &rows, qCountActive, ids); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.ProjectID] = row.N
	}
	return out, nil
}
