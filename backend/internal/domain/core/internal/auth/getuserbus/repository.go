package getuserbus

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

type userFinder interface {
	FindUserByID(ctx context.Context, id string) (base.User, bool, error)
	FindUserByEmail(ctx context.Context, email string) (base.User, bool, error)
	FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error)
}

// Repository membungkus store; hanya mengekspos pencarian user dan undangan.
type Repository struct{ s *store.Store }

func NewRepository(q db.Querier) *Repository { return &Repository{s: store.New(q)} }

func (r *Repository) FindUserByID(ctx context.Context, id string) (base.User, bool, error) {
	return r.s.FindUserByID(ctx, id)
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (base.User, bool, error) {
	return r.s.FindUserByEmail(ctx, email)
}

func (r *Repository) FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error) {
	return r.s.FindInvitation(ctx, userID)
}
