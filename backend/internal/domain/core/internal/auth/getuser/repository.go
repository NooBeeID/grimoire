package getuser

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

// userFinder dipakai usecase; diimplementasi Repository.
type userFinder interface {
	FindUserByID(ctx context.Context, id string) (base.User, bool, error)
}

// Repository membungkus store; hanya mengekspos pencarian user by id.
type Repository struct{ s *store.Store }

// NewRepository membuat Repository.
func NewRepository(q db.Querier) *Repository { return &Repository{s: store.New(q)} }

// FindUserByID mencari user; id bukan uuid dikembalikan (zero,false,nil) oleh store.
func (r *Repository) FindUserByID(ctx context.Context, id string) (base.User, bool, error) {
	return r.s.FindUserByID(ctx, id)
}
