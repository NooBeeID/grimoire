package getuser

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/auth/base"
)

// Usecase mengorkestrasi pembacaan detail user.
type Usecase struct {
	repo  userFinder
	clock base.Clock
}

// NewUsecase membuat Usecase.
func NewUsecase(repo userFinder, clock base.Clock) *Usecase {
	if clock == nil {
		clock = base.SystemClock
	}
	return &Usecase{repo: repo, clock: clock}
}

// Execute: id tidak ada / bukan uuid -> base.ErrUserNotFound (404).
func (u *Usecase) Execute(ctx context.Context, id string) (base.UserDTO, error) {
	user, found, err := u.repo.FindUserByID(ctx, id)
	if err != nil {
		return base.UserDTO{}, fmt.Errorf("getuser: find: %w", err)
	}
	if !found {
		return base.UserDTO{}, base.ErrUserNotFound
	}
	inv, hasInv, err := u.repo.FindInvitation(ctx, user.ID)
	if err != nil {
		return base.UserDTO{}, fmt.Errorf("getuser: find invitation: %w", err)
	}
	var invPtr *base.Invitation
	if hasInv {
		invPtr = &inv
	}
	return user.ToResponseWithInvitation(invPtr, u.clock()), nil
}
