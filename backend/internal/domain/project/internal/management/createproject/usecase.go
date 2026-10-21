package createproject

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

// Tx adalah operasi tulis di dalam satu transaksi.
type Tx interface {
	InsertProject(ctx context.Context, p base.Project) (base.Project, error)
	InsertMember(ctx context.Context, m base.Member) error
}

// Repository menjalankan fn dalam satu transaksi (rollback bila fn error).
type Repository interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
	repo  Repository
	audit audit.Recorder
	clock base.Clock
}

func NewUsecase(r Repository, a audit.Recorder, c base.Clock) *Usecase {
	return &Usecase{repo: r, audit: a, clock: c}
}

func ptr(i int) *int { return &i }

// Execute membuat project baru; developer otomatis menjadi owner, super_admin tidak.
func (u *Usecase) Execute(ctx context.Context, in CreateRequest, meta base.RequestMeta) (base.ProjectSummary, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return base.ProjectSummary{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if actor.Role != resolvesession.RoleDeveloper && actor.Role != resolvesession.RoleSuperAdmin {
		return base.ProjectSummary{}, base.ErrForbidden
	}
	now := u.clock()
	p, err := base.NewProject(in.Name, in.Description, actor, now)
	if err != nil {
		return base.ProjectSummary{}, err
	}

	memberRole, memberCount := "", 0
	err = u.repo.InTx(ctx, func(tx Tx) error {
		created, err := tx.InsertProject(ctx, p)
		if err != nil {
			return fmt.Errorf("createproject: insert project: %w", err)
		}
		p = created
		if actor.Role == resolvesession.RoleDeveloper {
			m, err := base.NewMember(p.ID, actor.ID, base.RoleOwner, actor, now)
			if err != nil {
				return fmt.Errorf("createproject: build owner: %w", err)
			}
			if err := tx.InsertMember(ctx, m); err != nil {
				return fmt.Errorf("createproject: insert owner: %w", err)
			}
			memberRole, memberCount = base.RoleOwner, 1
		}
		return nil
	})
	if err != nil {
		return base.ProjectSummary{}, err
	}

	level := base.ResolveLevel(actor.Role, memberRole, true)
	u.audit.Record(ctx, audit.Event{
		Action: record.ActionProjectCreated, ProjectID: p.ID, ProjectLabel: p.Name,
		TargetType: "project", Target: p.ID, Args: map[string]any{"name": p.Name}, Meta: meta,
	})
	return base.NewSummary(p, level, memberRole, memberCount,
		base.ContentCounts{Files: ptr(0), Specs: ptr(0), Keys: ptr(0)}), nil
}
