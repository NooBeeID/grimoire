package revokekey

import (
	"context"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/db"
)

// Repo adalah akses SQL feature revokekey.
type Repo struct {
	db *db.DB
	s  *store.Store
}

func newRepo(d *db.DB, s *store.Store) *Repo { return &Repo{db: d, s: s} }

var _ repository = (*Repo)(nil)

func (r *Repo) InTx(ctx context.Context, fn func(txRepo) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepoImpl{s: r.s.WithTx(tx)})
	})
}

type txRepoImpl struct{ s *store.Store }

var _ txRepo = (*txRepoImpl)(nil)

const lockInProjectSQL = `SELECT ` + store.KeyColumns + ` FROM api_keys WHERE id = $1 AND project_id = $2 FOR UPDATE`

const lockOwnedInProjectSQL = `SELECT ` + store.KeyColumns +
	` FROM api_keys WHERE id = $1 AND project_id = $2 AND created_by = $3::uuid FOR UPDATE`

func (t *txRepoImpl) LockInProject(ctx context.Context, projectID, keyID string, scope base.OwnerScope) (base.Key, bool, error) {
	if !store.ValidUUID(keyID) || !store.ValidUUID(projectID) || scope.Empty() {
		return base.Key{}, false, nil
	}
	if scope.All {
		return t.s.GetKey(ctx, lockInProjectSQL, keyID, projectID)
	}
	if !store.ValidUUID(scope.OwnerID) {
		return base.Key{}, false, nil
	}
	return t.s.GetKey(ctx, lockOwnedInProjectSQL, keyID, projectID, scope.OwnerID)
}

const markRevokedSQL = `UPDATE api_keys SET revoked_at = $2, revoked_by = $3, revoked_by_label = $4
WHERE id = $1 AND revoked_at IS NULL RETURNING ` + store.KeyColumns

// MarkRevoked memakai nilai revoked_* dari entity. Tidak ada baris ter-update -> ErrAlreadyRevoked.
func (t *txRepoImpl) MarkRevoked(ctx context.Context, k base.Key) (base.Key, error) {
	var by, label any
	if k.RevokedBy != nil {
		by = *k.RevokedBy
	}
	if k.RevokedByLabel != nil {
		label = *k.RevokedByLabel
	}
	out, found, err := t.s.GetKey(ctx, markRevokedSQL, k.ID, k.RevokedAt, by, label)
	if err != nil {
		return base.Key{}, err
	}
	if !found {
		return base.Key{}, base.ErrAlreadyRevoked
	}
	return out, nil
}
