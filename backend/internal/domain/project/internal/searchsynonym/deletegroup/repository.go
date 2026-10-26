package deletegroup

import (
	"context"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
)

// Tx adalah operasi yang berjalan dalam satu transaksi.
type Tx interface {
	LockProject(ctx context.Context, projectID string) error
	Delete(ctx context.Context, projectID, id string) (base.Group, bool, error)
}

// Repository adalah satu-satunya lapisan yang menyentuh SQL (lewat store bersama).
type Repository interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

type repo struct{ s *store.Store }

var _ Repository = (*repo)(nil)

func newRepo(s *store.Store) *repo { return &repo{s: s} }

// NewRepository dipakai tes integrasi.
func NewRepository(s *store.Store) Repository { return newRepo(s) }

func (r *repo) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.s.InTx(ctx, func(s *store.Store) error { return fn(s) })
}
