// Package addmember melayani POST /api/projects/{projectID}/members.
package addmember

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

type Tx interface {
	LockProject(ctx context.Context, id string) (base.Project, bool, error)
	LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
	InsertMember(ctx context.Context, m base.Member) (inserted bool, err error) // ON CONFLICT DO NOTHING
}

type Repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
	InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
	repo  Repository
	users users.Lookup
	audit audit.Recorder
	clock base.Clock
}

func NewUsecase(r Repository, u users.Lookup, a audit.Recorder, c base.Clock) *Usecase {
	return &Usecase{repo: r, users: u, audit: a, clock: c}
}

// Execute menambah satu developer aktif sebagai anggota. Bus get-user dipanggil sebelum tx,
// audit setelah commit; tidak ada panggilan bus di dalam tx.
func (u *Usecase) Execute(ctx context.Context, projectID string, in AddMemberRequest, meta base.RequestMeta) (base.MemberDTO, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return base.MemberDTO{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	_, memberRole, found, err := u.repo.FindAccess(ctx, projectID, actor.ID)
	if err != nil {
		return base.MemberDTO{}, fmt.Errorf("addmember: find access: %w", err)
	}
	if !found {
		return base.MemberDTO{}, base.ErrProjectNotFound
	}
	if err := base.Require(base.ResolveLevel(actor.Role, memberRole, true), base.PermManageMembers); err != nil {
		return base.MemberDTO{}, err
	}

	draft, err := base.NewMember(projectID, in.UserID, in.Role, actor, u.clock())
	if err != nil {
		return base.MemberDTO{}, err
	}

	target, err := u.users.Get(ctx, in.UserID)
	if err != nil {
		return base.MemberDTO{}, fmt.Errorf("addmember: get user: %w", err)
	}
	if err := base.ValidateCandidate(target); err != nil {
		return base.MemberDTO{}, err
	}
	draft.UserID = target.ID

	var project base.Project
	err = u.repo.InTx(ctx, func(tx Tx) error {
		p, ok, err := tx.LockProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("addmember: lock project: %w", err)
		}
		if !ok {
			return base.ErrProjectNotFound
		}
		project = p
		ms, err := tx.LockMembers(ctx, projectID)
		if err != nil {
			return fmt.Errorf("addmember: lock members: %w", err)
		}
		mine, _ := base.FindMember(ms, actor.ID)
		if err := base.Require(base.ResolveLevel(actor.Role, mine.Role, true), base.PermManageMembers); err != nil {
			return err
		}
		if _, exists := base.FindMember(ms, draft.UserID); exists {
			return base.ErrAlreadyMember
		}
		inserted, err := tx.InsertMember(ctx, draft)
		if err != nil {
			return fmt.Errorf("addmember: insert member: %w", err)
		}
		if !inserted {
			return base.ErrAlreadyMember
		}
		return nil
	})
	if err != nil {
		return base.MemberDTO{}, err
	}

	u.audit.Record(ctx, audit.Event{
		Action: record.ActionProjectMemberAdded, ProjectID: project.ID, ProjectLabel: project.Name,
		TargetType: "project_member", Target: draft.UserID,
		Args: map[string]any{"user_id": draft.UserID, "user_label": target.Email, "role": draft.Role},
		Meta: meta,
	})
	return draft.ToDTO(target), nil
}
