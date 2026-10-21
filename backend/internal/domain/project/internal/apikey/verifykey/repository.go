package verifykey

import (
	"context"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
)

type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

const qFindByPrefix = `SELECT ` + store.KeyColumns + ` FROM api_keys WHERE prefix = $1`

func (r *Repo) FindByPrefix(ctx context.Context, prefix string) (base.Key, bool, error) {
	return r.s.GetKey(ctx, qFindByPrefix, prefix)
}
