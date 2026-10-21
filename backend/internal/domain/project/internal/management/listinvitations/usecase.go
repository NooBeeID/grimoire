// Package listinvitations melayani GET /api/projects/{projectID}/invitations (undangan belum diterima).
package listinvitations

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type AccessFinder interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
}

type MemberLister interface {
	ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
}

type Usecase struct {
	access  AccessFinder
	members MemberLister
	users   users.Lookup
}

func NewUsecase(a AccessFinder, m MemberLister, u users.Lookup) *Usecase {
	return &Usecase{access: a, members: m, users: u}
}

type Result struct{ Items []base.InvitationDTO }

// Execute: owner/super_admin saja (member 403; non-anggota atau project tidak ada 404).
// Kegagalan get-user untuk satu anggota menggagalkan seluruh permintaan (tanpa hasil parsial).
// Debt: satu get-user per anggota (N+1), sama dengan listmembers; belum ada bus batch.
func (u *Usecase) Execute(ctx context.Context, projectID string) (Result, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	_, memberRole, found, err := u.access.FindAccess(ctx, projectID, actor.ID)
	if err != nil {
		return Result{}, fmt.Errorf("listinvitations: find access: %w", err)
	}
	if !found {
		return Result{}, base.ErrProjectNotFound
	}
	level := base.ResolveLevel(actor.Role, memberRole, true)
	if err := base.Require(level, base.PermManageMembers); err != nil {
		return Result{}, err
	}
	ms, err := u.members.ListMembers(ctx, projectID)
	if err != nil {
		return Result{}, fmt.Errorf("listinvitations: list members: %w", err)
	}
	items := make([]base.InvitationDTO, 0)
	for _, m := range ms {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("listinvitations: %w", err)
		}
		usr, err := u.users.Get(ctx, m.UserID)
		if err != nil {
			return Result{}, fmt.Errorf("listinvitations: get-user %s: %w", m.UserID, err)
		}
		if dto, ok := m.ToInvitationDTO(usr); ok {
			items = append(items, dto)
		}
	}
	base.SortInvitations(items)
	return Result{Items: items}, nil
}
