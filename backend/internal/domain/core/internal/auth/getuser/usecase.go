package getuser

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/auth/base"
)

// Usecase mengorkestrasi pembacaan detail user.
type Usecase struct{ repo userFinder }

// NewUsecase membuat Usecase.
func NewUsecase(repo userFinder) *Usecase { return &Usecase{repo: repo} }

// Execute: id tidak ada / bukan uuid -> base.ErrUserNotFound (404).
func (u *Usecase) Execute(ctx context.Context, id string) (base.UserDTO, error) {
	user, found, err := u.repo.FindUserByID(ctx, id)
	if err != nil {
		return base.UserDTO{}, fmt.Errorf("getuser: find: %w", err)
	}
	if !found {
		return base.UserDTO{}, base.ErrUserNotFound
	}
	return user.ToResponse(), nil
}
