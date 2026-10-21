// Package changememberrole melayani PATCH /api/projects/{projectID}/members/{userID} (ubah peran anggota).
package changememberrole

import (
	"context"
	"fmt"
	"log/slog"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/getuser"
)

// Repository adalah operasi di luar transaksi (sisi consumer).
type Repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
	InTx(ctx context.Context, fn func(TxRepository) error) error
}

// TxRepository adalah operasi di dalam satu transaksi.
type TxRepository interface {
	LockProject(ctx context.Context, id string) (base.Project, bool, error)
	LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
	UpdateMemberRole(ctx context.Context, projectID, userID, role string) error
}

type Input struct {
	ProjectID string
	UserID    string // dari path, apa adanya
	Role      string
	Meta      base.RequestMeta
}

type Usecase struct {
	repo  Repository
	users users.Lookup
	audit audit.Recorder
	log   *slog.Logger
}

func NewUsecase(repo Repository, u users.Lookup, a audit.Recorder, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{repo: repo, users: u, audit: a, log: log}
}

// Execute mengubah peran anggota. No-op (role sama) tidak menulis dan tidak mengaudit.
func (u *Usecase) Execute(ctx context.Context, in Input) (base.MemberDTO, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return base.MemberDTO{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	_, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)
	if err != nil {
		return base.MemberDTO{}, fmt.Errorf("changememberrole: cari akses: %w", err)
	}
	if !found {
		return base.MemberDTO{}, base.ErrProjectNotFound
	}
	level := base.ResolveLevel(actor.Role, memberRole, true)
	if err := base.Require(level, base.PermManageMembers); err != nil {
		return base.MemberDTO{}, err
	}

	var (
		project base.Project
		target  base.Member
		newRole string
		changed bool
	)
	err = u.repo.InTx(ctx, func(tx TxRepository) error {
		p, ok, err := tx.LockProject(ctx, in.ProjectID)
		if err != nil {
			return fmt.Errorf("changememberrole: kunci project: %w", err)
		}
		if !ok {
			return base.ErrProjectNotFound
		}
		ms, err := tx.LockMembers(ctx, p.ID)
		if err != nil {
			return fmt.Errorf("changememberrole: kunci anggota: %w", err)
		}
		// hitung ulang level aktor dari baris terkunci
		mine, _ := base.FindMember(ms, actor.ID)
		lvl := base.ResolveLevel(actor.Role, mine.Role, true)
		if err := base.Require(lvl, base.PermManageMembers); err != nil {
			return err
		}
		t, ok := base.FindMember(ms, in.UserID)
		if !ok {
			return base.ErrMemberNotFound
		}
		if err := base.CheckRoleChange(actor.ID, lvl, t, in.Role, base.OwnerCount(ms)); err != nil {
			return err
		}
		project, target, newRole = p, t, in.Role
		if in.Role == t.Role {
			return nil
		}
		if err := tx.UpdateMemberRole(ctx, p.ID, t.UserID, in.Role); err != nil {
			return fmt.Errorf("changememberrole: simpan: %w", err)
		}
		changed = true
		return nil
	})
	if err != nil {
		return base.MemberDTO{}, err
	}

	user, err := u.users.Get(ctx, target.UserID)
	if err != nil {
		u.log.WarnContext(ctx, "changememberrole: get-user gagal",
			"project_id", project.ID, "user_id", target.UserID, "error", err)
		user = getuser.Response{}
	}

	if changed {
		u.audit.Record(ctx, audit.Event{
			Action: record.ActionProjectMemberRoleChanged, ProjectID: project.ID, ProjectLabel: project.Name,
			TargetType: "project_member", Target: target.UserID,
			Args: map[string]any{
				"user_id": target.UserID, "user_label": user.Email,
				"role_from": target.Role, "role_to": newRole,
			},
			Meta: in.Meta,
		})
	}

	target.Role = newRole
	return target.ToDTO(user), nil
}
