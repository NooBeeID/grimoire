package creategroup

import (
	"context"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
)

// Tx adalah operasi yang berjalan dalam satu transaksi.
type Tx interface {
	LockProject(ctx context.Context, projectID string) error
	Count(ctx context.Context, projectID string) (int, error)
	TermsInUse(ctx context.Context, projectID string, terms []string, exceptID string) ([]string, error)
	Insert(ctx context.Context, g base.Group) (base.Group, error)
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
