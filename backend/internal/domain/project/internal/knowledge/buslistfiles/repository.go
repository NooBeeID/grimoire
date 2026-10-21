package buslistfiles

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
)

type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

type metaRow struct {
	Path      string    `db:"path"`
	SizeBytes int       `db:"size_bytes"`
	UpdatedAt time.Time `db:"updated_at"`
}

const listPageSQL = `
SELECT path, size_bytes, updated_at
FROM knowledge_files
WHERE project_id = $1 AND path LIKE $2 ESCAPE '\'
ORDER BY path COLLATE "C"
LIMIT $3 OFFSET $4`

const countSQL = `
SELECT count(*) FROM knowledge_files
WHERE project_id = $1 AND path LIKE $2 ESCAPE '\'`

func (r *Repo) ListMeta(ctx context.Context, projectID, likePrefix string) ([]base.Meta, error) {
	return r.s.ListMeta(ctx, projectID, likePrefix)
}

func (r *Repo) ListPage(ctx context.Context, projectID, likePrefix string, limit, offset int) ([]base.Meta, error) {
	if !store.ValidUUID(projectID) {
		return []base.Meta{}, nil
	}
	var rows []metaRow
	if err := r.s.Q().SelectContext(ctx, &rows, listPageSQL, projectID, likePrefix, limit, offset); err != nil {
		return nil, fmt.Errorf("buslistfiles: select page: %w", err)
	}
	out := make([]base.Meta, 0, len(rows))
	for _, row := range rows {
		out = append(out, base.Meta{Path: row.Path, SizeBytes: row.SizeBytes, UpdatedAt: row.UpdatedAt.UTC()})
	}
	return out, nil
}

func (r *Repo) CountByPrefix(ctx context.Context, projectID, likePrefix string) (int, error) {
	if !store.ValidUUID(projectID) {
		return 0, nil
	}
	var n int
	if err := r.s.Q().GetContext(ctx, &n, countSQL, projectID, likePrefix); err != nil {
		return 0, fmt.Errorf("buslistfiles: count: %w", err)
	}
	return n, nil
}
