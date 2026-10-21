package deletefolder

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
)

const deleteSQL = `WITH d AS (
  DELETE FROM knowledge_files
  WHERE project_id = $1 AND path LIKE $2 ESCAPE '\'
  RETURNING 1
)
SELECT count(*) FROM d`

// Repository adalah satu-satunya lapisan yang menyentuh SQL.
type Repository interface {
	// DeleteByPrefix menghapus semua baris project yang cocok dengan pola LIKE; mengembalikan jumlahnya.
	DeleteByPrefix(ctx context.Context, projectID, likePrefix string) (int, error)
}

type repo struct{ q db.Querier }

var _ Repository = (*repo)(nil)

func newRepo(s *store.Store) *repo { return &repo{q: s.Q()} }

func (r *repo) DeleteByPrefix(ctx context.Context, projectID, likePrefix string) (int, error) {
	if !store.ValidUUID(projectID) {
		return 0, nil
	}
	var n int
	if err := r.q.GetContext(ctx, &n, deleteSQL, projectID, likePrefix); err != nil {
		return 0, fmt.Errorf("deletefolder: delete: %w", err)
	}
	return n, nil
}
