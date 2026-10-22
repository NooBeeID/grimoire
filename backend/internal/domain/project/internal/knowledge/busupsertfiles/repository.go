package busupsertfiles

import (
	"context"

	"grimoire/internal/domain/project/internal/knowledge/base/ingest"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
)

type repository interface {
	InTx(ctx context.Context, fn func(ingest.Tx) error) error
}

// Repo membungkus *store.Store; tidak ada SQL di feature ini.
type Repo struct{ s *store.Store }

var _ repository = (*Repo)(nil)

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) InTx(ctx context.Context, fn func(ingest.Tx) error) error {
	return r.s.InTx(ctx, func(ts *store.Store) error { return fn(ts) })
}
