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

// listByOwnerSQL: hanya key milik owner. Key legacy (created_by NULL) tidak cocok.
const listByOwnerSQL = `SELECT ` + listColumns + `
FROM api_keys
WHERE project_id = $1 AND created_by = $2::uuid
ORDER BY (revoked_at IS NULL) DESC, created_at DESC, api_keys.id`

// Repo adalah akses SQL untuk feature listkeys.
type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

// ListByProject mengembalikan key (aktif dulu) sesuai scope. Hasil kosong -> slice kosong non-nil.
// Fail-closed: projectID bukan uuid, scope kosong, atau OwnerID bukan uuid -> (kosong, nil) tanpa query.
func (r *Repo) ListByProject(ctx context.Context, projectID string, scope base.OwnerScope) ([]base.Key, error) {
	if !store.ValidUUID(projectID) || scope.Empty() {
		return []base.Key{}, nil
	}
	var (
		keys []base.Key
		err  error
	)
	if scope.All {
		keys, err = r.s.SelectKeys(ctx, listByProjectSQL, projectID)
	} else {
		if !store.ValidUUID(scope.OwnerID) {
			return []base.Key{}, nil
		}
		keys, err = r.s.SelectKeys(ctx, listByOwnerSQL, projectID, scope.OwnerID)
	}
	if err != nil {
		return nil, fmt.Errorf("listkeys: select keys: %w", err)
	}
	return keys, nil
}
