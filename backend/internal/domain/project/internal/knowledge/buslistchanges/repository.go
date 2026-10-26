package buslistchanges

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

type changeRow struct {
	Path      string    `db:"path"`
	SizeBytes int       `db:"size_bytes"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Total     int       `db:"total"`
}

// listChangesSQL memakai indeks knowledge_files_project_updated_idx (project_id, updated_at DESC).
const listChangesSQL = `
SELECT path, size_bytes, created_at, updated_at, count(*) OVER () AS total
FROM knowledge_files
WHERE project_id = $1 AND updated_at >= $2 AND path LIKE $3 ESCAPE '\'
ORDER BY updated_at DESC, path COLLATE "C"
LIMIT $4`

func (r *Repo) ListChanges(ctx context.Context, projectID string, since time.Time, likePrefix string, limit int) ([]listchanges.Item, int, error) {
	if !store.ValidUUID(projectID) {
		return []listchanges.Item{}, 0, nil
	}
	var rows []changeRow
	if err := r.s.Q().SelectContext(ctx, &rows, listChangesSQL, projectID, since, likePrefix, limit); err != nil {
		return nil, 0, fmt.Errorf("buslistchanges: select: %w", err)
	}
	items := make([]listchanges.Item, 0, len(rows))
	total := 0
	for _, row := range rows {
		total = row.Total
		items = append(items, listchanges.Item{
			Path: row.Path, SizeBytes: row.SizeBytes,
			CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
		})
	}
	return items, total, nil
}
