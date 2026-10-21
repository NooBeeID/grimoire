package listkeys

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
)

// listColumns = store.KeyColumns tanpa secret_hash. Hash tidak pernah dibaca pada jalur daftar.
const listColumns = "id::text AS id, project_id::text AS project_id, name, prefix, " +
	"created_by::text AS created_by, created_by_label, created_at, " +
	"last_used_at, revoked_at, revoked_by::text AS revoked_by, revoked_by_label"

const listByProjectSQL = `SELECT ` + listColumns + `
FROM api_keys
WHERE project_id = $1
ORDER BY (revoked_at IS NULL) DESC, created_at DESC, api_keys.id`

// Repo adalah akses SQL untuk feature listkeys.
type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

// ListByProject mengembalikan semua key (aktif dulu). Hasil kosong -> slice kosong non-nil.
// projectID bukan uuid -> (kosong, nil) tanpa query.
func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]base.Key, error) {
	if !store.ValidUUID(projectID) {
		return []base.Key{}, nil
	}
	keys, err := r.s.SelectKeys(ctx, listByProjectSQL, projectID)
	if err != nil {
		return nil, fmt.Errorf("listkeys: select keys: %w", err)
	}
	return keys, nil
}
