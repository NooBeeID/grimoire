// Package deleteproject melayani DELETE /api/projects/{projectID} (hapus permanen, cascade lewat FK).
package deleteproject

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

type txRepo interface {
	LockProject(ctx context.Context, id string) (base.Project, bool, error)
	LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
	DeleteProject(ctx context.Context, id string) (bool, error) // true bila 1 baris terhapus
}

type repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
	InTx(ctx context.Context, fn func(txRepo) error) error
}

type Input struct {
	ProjectID   string
	ConfirmName string
	Meta        base.RequestMeta
}

type Usecase struct {
	repo   repository
	counts counts.Counter
	audit  audit.Recorder
}

func NewUsecase(r repository, c counts.Counter, a audit.Recorder) *Usecase {
	return &Usecase{repo: r, counts: c, audit: a}
}

// Execute menghapus project. Bus (counts, audit) tidak pernah dipanggil di dalam tx.
func (u *Usecase) Execute(ctx context.Context, in Input) error {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return errs.New(errs.CodeUnauthorized, "perlu login")
	}
	p, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)
	if err != nil {
		return fmt.Errorf("deleteproject: find access: %w", err)
	}
	if !found {
		return base.ErrProjectNotFound
	}
	if err := base.Require(base.ResolveLevel(actor.Role, memberRole, true), base.PermDelete); err != nil {
		return err
	}
	if err := p.ConfirmDeletion(in.ConfirmName); err != nil {
		return err
	}
	cc := u.counts.ForProjects(ctx, []string{p.ID})[p.ID]

	var memberCount int
	var name string
	err = u.repo.InTx(ctx, func(tx txRepo) error {
		locked, ok, err := tx.LockProject(ctx, p.ID)
		if err != nil {
			return fmt.Errorf("deleteproject: lock project: %w", err)
		}
		if !ok {
			return base.ErrProjectNotFound
		}
		if err := locked.ConfirmDeletion(in.ConfirmName); err != nil {
			return err
		}
		ms, err := tx.LockMembers(ctx, p.ID)
		if err != nil {
			return fmt.Errorf("deleteproject: lock members: %w", err)
		}
		m, _ := base.FindMember(ms, actor.ID)
		if err := base.Require(base.ResolveLevel(actor.Role, m.Role, true), base.PermDelete); err != nil {
			return err
		}
		memberCount = len(ms)
		name = locked.Name
		deleted, err := tx.DeleteProject(ctx, p.ID)
		if err != nil {
			return fmt.Errorf("deleteproject: delete: %w", err)
		}
		if !deleted {
			return base.ErrProjectNotFound
		}
		return nil
	})
	if err != nil {
		var e *errs.Error
		if errors.As(err, &e) {
			return err
		}
		return fmt.Errorf("deleteproject: tx: %w", err)
	}

	u.audit.Record(ctx, audit.Event{
		Action: record.ActionProjectDeleted, ProjectID: p.ID, ProjectLabel: name,
		TargetType: "project", Target: p.ID, Args: deletedArgs(name, memberCount, cc), Meta: in.Meta,
	})
	return nil
}

func deletedArgs(name string, memberCount int, cc base.ContentCounts) map[string]any {
	return map[string]any{
		"name":         name,
		"member_count": memberCount,
		"counts":       map[string]any{"files": deref(cc.Files), "specs": deref(cc.Specs), "keys": deref(cc.Keys)},
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
