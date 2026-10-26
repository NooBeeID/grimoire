package listgroups

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/access"
)

type Usecase struct {
	access access.Checker
	repo   Repository
}

func NewUsecase(a access.Checker, r Repository) *Usecase { return &Usecase{access: a, repo: r} }

// Execute: access -> List. Hasil tidak pernah nil.
func (u *Usecase) Execute(ctx context.Context, projectID string) ([]base.GroupDTO, error) {
	if _, err := u.access.Require(ctx, projectID); err != nil {
		return nil, err
	}
	groups, err := u.repo.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listgroups: %w", err)
	}
	out := make([]base.GroupDTO, 0, len(groups))
	for _, g := range groups {
		out = append(out, g.ToDTO())
	}
	return out, nil
}
