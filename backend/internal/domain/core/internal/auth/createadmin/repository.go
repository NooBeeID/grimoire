package createadmin

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

// Repository adalah kebutuhan usecase terhadap penyimpanan.
type Repository interface {
	// InsertUser mengembalikan base.ErrEmailTaken bila email sudah ada.
	InsertUser(ctx context.Context, u base.User) (base.User, error)
}

type repo struct{ s *store.Store }

func newRepo(q db.Querier) *repo { return &repo{s: store.New(q)} }

func (r *repo) InsertUser(ctx context.Context, u base.User) (base.User, error) {
	return r.s.InsertUser(ctx, u)
}
