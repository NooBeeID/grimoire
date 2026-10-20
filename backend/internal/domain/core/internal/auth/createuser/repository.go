package createuser

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

// Repo mendelegasikan insert ke query bersama di base/store.
type Repo struct{ s *store.Store }

// NewRepo membuat Repo.
func NewRepo(q db.Querier) *Repo { return &Repo{s: store.New(q)} }

// Insert menyimpan user; base.ErrEmailTaken atau error infra dikembalikan apa adanya.
func (r *Repo) Insert(ctx context.Context, u base.User) (base.User, error) {
	return r.s.InsertUser(ctx, u)
}
