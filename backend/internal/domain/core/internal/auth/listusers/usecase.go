package listusers

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/auth/base"
)

// Input adalah nilai mentah filter dari transport.
type Input = base.UserFilterInput

// Output adalah respons daftar user.
type Output struct {
	Items  []base.UserDTO `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// Usecase mengorkestrasi pembacaan daftar user.
type Usecase struct {
	repo  Repository
	clock base.Clock
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, clock base.Clock) *Usecase {
	if clock == nil {
		clock = base.SystemClock
	}
	return &Usecase{repo: repo, clock: clock}
}

// Execute memvalidasi filter (400 bila gagal), mengambil satu halaman, dan memetakan ke DTO.
func (u *Usecase) Execute(ctx context.Context, in Input) (Output, error) {
	f, err := base.NewUserFilter(in)
	if err != nil {
		if fe := base.AsFieldErrors(err); len(fe) > 0 {
			return Output{}, base.InvalidFields(fe)
		}
		return Output{}, fmt.Errorf("listusers: filter: %w", err)
	}
	users, total, err := u.repo.List(ctx, f)
	if err != nil {
		return Output{}, fmt.Errorf("listusers: list: %w", err)
	}
	now := u.clock()
	items := make([]base.UserDTO, len(users))
	for i := range users {
		items[i] = users[i].User.ToResponseWithInvitation(users[i].Invitation, now)
	}
	return Output{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}
