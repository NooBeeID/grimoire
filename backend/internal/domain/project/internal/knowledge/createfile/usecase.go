// Package createfile membuat satu file knowledge baru dari editor web.
package createfile

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository adalah akses data createfile.
type Repository interface {
	InTx(ctx context.Context, fn func(Repository) error) error
	LockProject(ctx context.Context, projectID string) error
	CountFiles(ctx context.Context, projectID string) (int, error)
	// Insert memakai RETURNING; unique violation -> base.ErrPathExists.
	Insert(ctx context.Context, f base.File) (base.File, error)
}

type Usecase struct {
	repo   Repository
	access access.Checker
	audit  audit.Recorder
	limits base.Limits
}

func NewUsecase(repo Repository, ac access.Checker, au audit.Recorder, limits base.Limits) *Usecase {
	return &Usecase{repo: repo, access: ac, audit: au, limits: limits}
}

func (u *Usecase) Execute(ctx context.Context, projectID string, req CreateRequest, meta base.RequestMeta) (base.FileDTO, error) {
	actor, err := u.access.Require(ctx, projectID)
	if err != nil {
		return base.FileDTO{}, err
	}
	path, r := base.NormalizePath(req.Path)
	if r != "" {
		return base.FileDTO{}, base.Invalid(r)
	}
	if base.IsHidden(path) {
		return base.FileDTO{}, base.Invalid(base.ReasonHidden)
	}
	if !base.IsSupported(path) {
		return base.FileDTO{}, base.Invalid(base.ReasonUnsupportedType)
	}
	content, r := base.PrepareContent([]byte(req.Content), u.limits.MaxFileBytes)
	if r == base.ReasonTooLarge {
		return base.FileDTO{}, base.TooLarge(r, u.limits.MaxFileBytes)
	}
	if r != "" {
		return base.FileDTO{}, base.Invalid(r)
	}
	f := base.NewFile(projectID, path, content, actor)

	var saved base.File
	err = u.repo.InTx(ctx, func(tx Repository) error {
		if err := tx.LockProject(ctx, projectID); err != nil {
			return fmt.Errorf("createfile: lock project: %w", err)
		}
		n, err := tx.CountFiles(ctx, projectID)
		if err != nil {
			return fmt.Errorf("createfile: count files: %w", err)
		}
		if n >= u.limits.MaxProjectFiles {
			return base.Invalid(base.ReasonProjectFileLimit)
		}
		s, err := tx.Insert(ctx, f)
		if err != nil {
			if err == base.ErrPathExists {
				return err
			}
			return fmt.Errorf("createfile: insert: %w", err)
		}
		saved = s
		return nil
	})
	if err != nil {
		return base.FileDTO{}, err
	}

	u.audit.Record(ctx, audit.Event{
		Action: record.ActionKnowledgeFileCreated, ProjectID: projectID, Actor: actor,
		TargetType: audit.TargetFile, Target: saved.Path,
		Args: map[string]any{}, Result: map[string]any{"bytes": saved.SizeBytes}, Meta: meta,
	})
	return saved.ToDTO(), nil
}
