package renamespec

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/lib/contracts/core/audit/record"
)

type Tx interface {
	LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
	RenameSpec(ctx context.Context, spec base.Spec) error
	FindSpecMeta(ctx context.Context, projectID, name string) (base.SpecSummary, bool, error)
}

type Repository interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
	repo  Repository
	guard access.Checker
	audit audit.Recorder
	now   base.Clock
}

func NewUsecase(repo Repository, guard access.Checker, rec audit.Recorder, now base.Clock) *Usecase {
	if now == nil {
		now = base.SystemClock
	}
	return &Usecase{repo: repo, guard: guard, audit: rec, now: now}
}

type Input struct {
	ProjectID string
	OldName   string
	NewName   string
	Actor     base.Actor
	Meta      base.RequestMeta
}

func (u *Usecase) Execute(ctx context.Context, in Input) (base.SpecSummary, error) {
	var zero base.SpecSummary
	if err := base.ValidateName(in.NewName); err != nil {
		return zero, err
	}
	if err := u.guard.RequireMember(ctx, in.ProjectID); err != nil {
		return zero, err
	}
	renamed := false
	var out base.SpecSummary
	err := u.repo.InTx(ctx, func(tx Tx) error {
		spec, ok, err := tx.LockSpec(ctx, in.ProjectID, in.OldName)
		if err != nil {
			return fmt.Errorf("renamespec: lock: %w", err)
		}
		if !ok {
			return base.ErrSpecNotFound
		}
		renamed = spec.Rename(in.NewName, in.Actor, u.now())
		if renamed {
			if err := tx.RenameSpec(ctx, spec); err != nil {
				if errors.Is(err, store.ErrNameTaken) {
					return base.ErrNameTaken
				}
				return fmt.Errorf("renamespec: rename: %w", err)
			}
		}
		sum, ok, err := tx.FindSpecMeta(ctx, in.ProjectID, spec.Name)
		if err != nil {
			return fmt.Errorf("renamespec: find meta: %w", err)
		}
		if !ok {
			return errors.New("renamespec: find meta: spec hilang dalam tx")
		}
		out = sum
		return nil
	})
	if err != nil {
		return zero, err
	}
	if renamed {
		u.audit.Record(ctx, audit.Event{
			Action: record.ActionAPISpecUpdated, ProjectID: in.ProjectID, Target: in.NewName,
			Args: map[string]any{"from": in.OldName, "to": in.NewName}, Meta: in.Meta,
		})
	}
	return out, nil
}
