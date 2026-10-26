// Package buslistgroups menyediakan handler bus project.searchsynonym.list-groups (consumer T20 knowledge).
package buslistgroups

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/lib/contracts/project/searchsynonym/listgroups"
)

type Repository interface {
	ListGroups(ctx context.Context, projectID string) ([][]string, error)
}

type Usecase struct{ repo Repository }

func New(repo Repository) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, in listgroups.Request) (listgroups.Response, error) {
	pid, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return listgroups.Response{}, base.Invalid(base.ReasonInvalidProject)
	}
	groups, err := u.repo.ListGroups(ctx, pid.String())
	if err != nil {
		return listgroups.Response{}, fmt.Errorf("buslistgroups: list: %w", err)
	}
	if len(groups) == 0 {
		groups = nil
	}
	return listgroups.Response{Groups: groups}, nil
}
