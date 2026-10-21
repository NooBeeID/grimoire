package deletespec

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

type txRepo interface {
	LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
	CountEndpoints(ctx context.Context, specID string) (int, error)
	// DeleteSpec true bila tepat 1 baris terhapus (endpoint ikut cascade FK).
	DeleteSpec(ctx context.Context, projectID, specID string) (bool, error)
}

type repository interface {
	InTx(ctx context.Context, fn func(txRepo) error) error
}

type Usecase struct {
	repo  repository
	guard access.Checker
	audit audit.Recorder
}

func NewUsecase(repo repository, guard access.Checker, rec audit.Recorder) *Usecase {
	return &Usecase{repo: repo, guard: guard, audit: rec}
}

func (u *Usecase) Execute(ctx context.Context, p Params, meta base.RequestMeta) error {
	if _, ok := base.ActorFrom(reqctx.IdentityFrom(ctx)); !ok {
		return errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if err := u.guard.RequireMember(ctx, p.ProjectID); err != nil {
		return err
	}
	var (
		name  string
		count int
	)
	err := u.repo.InTx(ctx, func(tx txRepo) error {
		sp, found, err := tx.LockSpec(ctx, p.ProjectID, p.Name)
		if err != nil {
			return fmt.Errorf("deletespec: lock spec: %w", err)
		}
		if !found {
			return base.ErrSpecNotFound
		}
		if count, err = tx.CountEndpoints(ctx, sp.ID); err != nil {
			return fmt.Errorf("deletespec: count endpoints: %w", err)
		}
		deleted, err := tx.DeleteSpec(ctx, p.ProjectID, sp.ID)
		if err != nil {
			return fmt.Errorf("deletespec: delete spec: %w", err)
		}
		if !deleted {
			return base.ErrSpecNotFound
		}
		name = sp.Name
		return nil
	})
	if err != nil {
		return err
	}
	u.audit.Record(ctx, audit.Event{
		Action:    record.ActionAPISpecDeleted,
		ProjectID: p.ProjectID,
		Target:    name,
		Args:      map[string]any{"name": name, "endpoint_count": count},
		Meta:      meta,
	})
	return nil
}
