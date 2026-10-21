// Package listmembers melayani GET /api/projects/{projectID}/members (daftar anggota diperkaya get-user).
package listmembers

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

type Result struct{ Items []base.MemberDTO }

// Execute: bukan anggota, project tidak ada, dan id tidak valid semuanya ErrProjectNotFound.
// Kegagalan get-user untuk satu anggota menggagalkan seluruh permintaan (data wajib lengkap).
func (u *Usecase) Execute(ctx context.Context, projectID string) (Result, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	_, memberRole, found, err := u.access.FindAccess(ctx, projectID, actor.ID)
	if err != nil {
		return Result{}, fmt.Errorf("listmembers: find access: %w", err)
	}
	if !found {
		return Result{}, base.ErrProjectNotFound
	}
	level := base.ResolveLevel(actor.Role, memberRole, true)
	if err := base.Require(level, base.PermView); err != nil {
		return Result{}, err
	}
	ms, err := u.members.ListMembers(ctx, projectID)
	if err != nil {
		return Result{}, fmt.Errorf("listmembers: list members: %w", err)
	}
	items := make([]base.MemberDTO, 0, len(ms))
	for _, m := range ms {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("listmembers: %w", err)
		}
		usr, err := u.users.Get(ctx, m.UserID)
		if err != nil {
			return Result{}, fmt.Errorf("listmembers: get-user %s: %w", m.UserID, err)
		}
		items = append(items, m.ToDTO(usr))
	}
	base.SortMembers(items)
	return Result{Items: items}, nil
}
