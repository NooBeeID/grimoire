package countfiles

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
)

// Repository adalah akses SQL untuk feature countfiles.
type Repository struct{ s *store.Store }

func NewRepository(s *store.Store) *Repository { return &Repository{s: s} }

type countRow struct {
	ProjectID string `db:"project_id"`
	N         int64  `db:"n"`
}

const countSQL = `
SELECT project_id::text AS project_id, count(*) AS n
FROM knowledge_files
WHERE project_id = ANY($1::text[]::uuid[])
GROUP BY project_id`

// CountByProject mengembalikan jumlah file per project untuk uuid KANONIK (lowercase).
// Project tanpa file tidak muncul di map (usecase mengisi 0).
func (r *Repository) CountByProject(ctx context.Context, projectIDs []string) (map[string]int, error) {
	var rows []countRow
	if err := r.s.Q().SelectContext(ctx, &rows, countSQL, projectIDs); err != nil {
		return nil, fmt.Errorf("countfiles: count: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.ProjectID] = int(row.N)
	}
	return out, nil
}
