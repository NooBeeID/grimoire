package getuserbus

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

type userFinder interface {
	FindUserByID(ctx context.Context, id string) (base.User, bool, error)
}

// Repository membungkus store; hanya mengekspos pencarian user by id.
type Repository struct{ s *store.Store }

func NewRepository(q db.Querier) *Repository { return &Repository{s: store.New(q)} }

func (r *Repository) FindUserByID(ctx context.Context, id string) (base.User, bool, error) {
	return r.s.FindUserByID(ctx, id)
}
