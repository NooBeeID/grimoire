// Package getproject melayani GET /api/projects/{projectID} (detail project).
package getproject

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error)
	MemberStats(ctx context.Context, projectID string) (members, owners int, err error)
}

type Usecase struct {
	repo   repository
	counts counts.Counter
}

func NewUsecase(repo repository, c counts.Counter) *Usecase { return &Usecase{repo: repo, counts: c} }

// Execute: bukan anggota, tidak ada, dan id tidak valid semuanya ErrProjectNotFound (tidak bocor).
func (u *Usecase) Execute(ctx context.Context, projectID string) (base.ProjectDetail, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return base.ProjectDetail{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	p, memberRole, found, err := u.repo.FindAccess(ctx, projectID, actor.ID)
	if err != nil {
		return base.ProjectDetail{}, fmt.Errorf("getproject: find access: %w", err)
	}
	if !found {
		return base.ProjectDetail{}, base.ErrProjectNotFound
	}
	level := base.ResolveLevel(actor.Role, memberRole, true)
	if err := base.Require(level, base.PermView); err != nil {
		return base.ProjectDetail{}, err
	}
	members, owners, err := u.repo.MemberStats(ctx, p.ID)
	if err != nil {
		return base.ProjectDetail{}, fmt.Errorf("getproject: member stats: %w", err)
	}
	c := u.counts.ForProjects(ctx, []string{p.ID})[p.ID]
	return base.ProjectDetail{
		ProjectSummary: base.NewSummary(p, level, memberRole, members, c),
		OwnerCount:     owners,
	}, nil
}
