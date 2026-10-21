package deletefile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
)

const deleteSQL = `DELETE FROM knowledge_files
WHERE project_id = $1 AND path = $2
RETURNING size_bytes`

// Repository adalah satu-satunya lapisan yang menyentuh SQL.
type Repository interface {
	// DeleteByPath menghapus satu baris. found=false bila 0 baris. sizeBytes = size_bytes baris yang dihapus.
	DeleteByPath(ctx context.Context, projectID, path string) (sizeBytes int, found bool, err error)
}

type repo struct{ q db.Querier }

var _ Repository = (*repo)(nil)

func newRepo(s *store.Store) *repo { return &repo{q: s.Q()} }

func (r *repo) DeleteByPath(ctx context.Context, projectID, path string) (int, bool, error) {
	if !store.ValidUUID(projectID) {
		return 0, false, nil
	}
	var size int
	if err := r.q.GetContext(ctx, &size, deleteSQL, projectID, path); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("deletefile: delete: %w", err)
	}
	return size, true, nil
}
