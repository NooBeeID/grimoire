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
type Usecase struct{ repo Repository }

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository) *Usecase { return &Usecase{repo: repo} }

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
	items := make([]base.UserDTO, len(users))
	for i := range users {
		items[i] = users[i].ToResponse()
	}
	return Output{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}
