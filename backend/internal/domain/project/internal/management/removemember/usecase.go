// Package removemember melayani DELETE /api/projects/{projectID}/members/{userID}
// (mengeluarkan anggota atau keluar sendiri).
package removemember

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

// Sisi consumer. Repo konkret memenuhi keduanya.
type Repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
	InTx(ctx context.Context, fn func(TxRepository) error) error
}

type TxRepository interface {
	LockProject(ctx context.Context, id string) (base.Project, bool, error)
	LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
	DeleteMember(ctx context.Context, projectID, userID string) error
}

type Input struct {
	ProjectID string
	UserID    string
	Meta      base.RequestMeta
}

type Usecase struct {
	repo  Repository
	users users.Lookup
	audit audit.Recorder
	log   *slog.Logger
}

func NewUsecase(repo Repository, u users.Lookup, a audit.Recorder, log *slog.Logger) *Usecase {
	return &Usecase{repo: repo, users: u, audit: a, log: log}
}

// Execute mengeluarkan anggota. Bus (get-user, audit) hanya dipanggil setelah commit.
func (u *Usecase) Execute(ctx context.Context, in Input) error {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return errs.New(errs.CodeUnauthorized, "perlu login")
	}
	_, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)
	if err != nil {
		return fmt.Errorf("removemember: cari akses: %w", err)
	}
	if !found {
		return base.ErrProjectNotFound
	}
	level := base.ResolveLevel(actor.Role, memberRole, true)
	if err := base.Require(level, base.PermView); err != nil {
		return err
	}
	targetID, err := uuid.Parse(in.UserID)
	if err != nil {
		return base.ErrMemberNotFound
	}
	target := targetID.String()

	var removed base.Member
	var project base.Project
	var self bool
	err = u.repo.InTx(ctx, func(tx TxRepository) error {
		p, ok, err := tx.LockProject(ctx, in.ProjectID)
		if err != nil {
			return fmt.Errorf("removemember: kunci project: %w", err)
		}
		if !ok {
			return base.ErrProjectNotFound
		}
		members, err := tx.LockMembers(ctx, p.ID)
		if err != nil {
			return fmt.Errorf("removemember: kunci anggota: %w", err)
		}
		myRole := ""
		if me, ok := base.FindMember(members, actor.ID); ok {
			myRole = me.Role
		}
		lvl := base.ResolveLevel(actor.Role, myRole, true)
		if err := base.Require(lvl, base.PermView); err != nil {
			return err
		}
		t, ok := base.FindMember(members, target)
		if !ok {
			return base.ErrMemberNotFound
		}
		if err := base.CheckRemove(actor.ID, lvl, t, base.OwnerCount(members)); err != nil {
			return err
		}
		if err := tx.DeleteMember(ctx, p.ID, t.UserID); err != nil {
			return fmt.Errorf("removemember: hapus anggota: %w", err)
		}
		project, removed, self = p, t, t.UserID == actor.ID
		return nil
	})
	if err != nil {
		return err
	}

	label := ""
	if res, err := u.users.Get(ctx, removed.UserID); err != nil {
		u.log.WarnContext(ctx, "removemember: get-user gagal, label audit kosong",
			"project_id", project.ID, "user_id", removed.UserID, "error", err)
	} else if res.Found {
		label = res.Email
	}
	u.audit.Record(ctx, audit.Event{
		Action: record.ActionProjectMemberRemoved, ProjectID: project.ID, ProjectLabel: project.Name,
		TargetType: "project_member", Target: removed.UserID,
		Args: map[string]any{"user_id": removed.UserID, "user_label": label, "role": removed.Role, "self": self},
		Meta: in.Meta,
	})
	return nil
}
