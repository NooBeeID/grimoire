// Package movefile memindahkan atau mengubah nama satu file knowledge dalam project yang sama.
package movefile

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository didefinisikan di sisi consumer; dipenuhi *Repo.
type Repository interface {
	InTx(ctx context.Context, fn func(Repository) error) error
	LockProject(ctx context.Context, projectID string) error
	PathExists(ctx context.Context, projectID, path string) (bool, error)
	Rename(ctx context.Context, projectID, from, to string, actor base.Actor) (int64, error)
	FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
}

type Usecase struct {
	access access.Checker
	repo   Repository
	audit  audit.Recorder
}

func NewUsecase(a access.Checker, r Repository, au audit.Recorder) *Usecase {
	return &Usecase{access: a, repo: r, audit: au}
}

// Execute: otorisasi, validasi murni, lalu rename dalam tx berlock, audit setelah commit.
func (u *Usecase) Execute(ctx context.Context, projectID string, in Request, meta base.RequestMeta) (base.FileDTO, error) {
	actor, err := u.access.Require(ctx, projectID)
	if err != nil {
		return base.FileDTO{}, err
	}
	from, r := base.NormalizePath(in.From)
	if r != "" {
		return base.FileDTO{}, base.Invalid(r)
	}
	to, r := base.NormalizePath(in.To)
	if r != "" {
		return base.FileDTO{}, base.Invalid(r)
	}
	if base.IsHidden(to) {
		return base.FileDTO{}, base.Invalid(base.ReasonHidden)
	}
	if !base.IsSupported(to) {
		return base.FileDTO{}, base.Invalid(base.ReasonUnsupportedType)
	}
	if from == to {
		return base.FileDTO{}, base.Invalid(base.ReasonSamePath)
	}

	var moved base.File
	err = u.repo.InTx(ctx, func(tx Repository) error {
		if err := tx.LockProject(ctx, projectID); err != nil {
			return fmt.Errorf("movefile: lock: %w", err)
		}
		ok, err := tx.PathExists(ctx, projectID, from)
		if err != nil {
			return fmt.Errorf("movefile: exists from: %w", err)
		}
		if !ok {
			return base.ErrFileNotFound
		}
		ok, err = tx.PathExists(ctx, projectID, to)
		if err != nil {
			return fmt.Errorf("movefile: exists to: %w", err)
		}
		if ok {
			return base.ErrPathExists
		}
		n, err := tx.Rename(ctx, projectID, from, to, actor)
		if err != nil {
			if errors.Is(err, base.ErrPathExists) {
				return err
			}
			return fmt.Errorf("movefile: rename: %w", err)
		}
		if n == 0 {
			return base.ErrFileNotFound
		}
		f, found, err := tx.FindByPath(ctx, projectID, to)
		if err != nil {
			return fmt.Errorf("movefile: reload: %w", err)
		}
		if !found {
			return fmt.Errorf("movefile: reload: file hilang setelah rename")
		}
		moved = f
		return nil
	})
	if err != nil {
		return base.FileDTO{}, err
	}

	u.audit.Record(ctx, audit.Event{
		Action: record.ActionKnowledgeFileMoved, ProjectID: projectID, Actor: actor,
		TargetType: audit.TargetFile, Target: to,
		Args:   map[string]any{"from": from, "to": to},
		Result: map[string]any{"bytes": moved.SizeBytes},
		Meta:   meta,
	})
	return moved.ToDTO(), nil
}
