package listprojects

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

type Input = ProjectFilterInput

type Output struct {
	Items  []base.ProjectSummary `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

type Usecase struct {
	repo   Repository
	counts counts.Counter
}

func NewUsecase(repo Repository, c counts.Counter) *Usecase {
	return &Usecase{repo: repo, counts: c}
}

func (u *Usecase) Execute(ctx context.Context, in Input) (Output, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return Output{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if actor.Role != resolvesession.RoleDeveloper && actor.Role != resolvesession.RoleSuperAdmin {
		return Output{}, base.ErrForbidden
	}
	f, err := NewProjectFilter(in)
	if err != nil {
		var fe base.FieldErrors
		if errors.As(err, &fe) {
			return Output{}, base.InvalidFields(fe)
		}
		return Output{}, err
	}
	items, total, err := u.repo.List(ctx, actor.ID, actor.Role == resolvesession.RoleDeveloper, f)
	if err != nil {
		return Output{}, fmt.Errorf("listprojects: list: %w", err)
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.Project.ID)
	}
	cm := u.counts.ForProjects(ctx, ids)
	out := make([]base.ProjectSummary, 0, len(items))
	for _, it := range items {
		level := base.ResolveLevel(actor.Role, it.MyRole, true)
		out = append(out, base.NewSummary(it.Project, level, it.MyRole, it.MemberCount, cm[it.Project.ID]))
	}
	return Output{Items: out, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}
