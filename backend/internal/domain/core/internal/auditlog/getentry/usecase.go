package getentry

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

// Repository adalah akses data yang dibutuhkan usecase.
type Repository interface {
	Get(ctx context.Context, id string) (entry base.Entry, found bool, err error)
}

// Usecase mengorkestrasi pembacaan satu entri audit.
type Usecase struct{ repo Repository }

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository) *Usecase { return &Usecase{repo: repo} }

// Get mengembalikan entri; tidak ketemu menjadi errs.ErrNotFound.
func (u *Usecase) Get(ctx context.Context, p Params) (base.Entry, error) {
	e, found, err := u.repo.Get(ctx, p.ID)
	if err != nil {
		return base.Entry{}, fmt.Errorf("get audit entry: %w", err)
	}
	if !found {
		return base.Entry{}, notFound()
	}
	return e, nil
}
