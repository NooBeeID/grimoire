package deletefile

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/lib/contracts/core/audit/record"
)

type Usecase struct {
	access access.Checker
	repo   Repository
	audit  audit.Recorder
}

func NewUsecase(a access.Checker, r Repository, au audit.Recorder) *Usecase {
	return &Usecase{access: a, repo: r, audit: au}
}

// Execute: access -> NormalizePath -> DeleteByPath -> audit (fail-open, setelah statement sukses).
func (u *Usecase) Execute(ctx context.Context, p params, meta base.RequestMeta) error {
	actor, err := u.access.Require(ctx, p.ProjectID)
	if err != nil {
		return err
	}
	path, reason := base.NormalizePath(p.Path)
	if reason != "" {
		return base.Invalid(reason)
	}
	size, found, err := u.repo.DeleteByPath(ctx, p.ProjectID, path)
	if err != nil {
		return fmt.Errorf("deletefile: %w", err)
	}
	if !found {
		return base.ErrFileNotFound
	}
	u.audit.Record(ctx, audit.Event{
		Action:     record.ActionKnowledgeFileDeleted,
		ProjectID:  p.ProjectID,
		Actor:      actor,
		TargetType: audit.TargetFile,
		Target:     path,
		Args:       map[string]any{},
		Result:     map[string]any{"bytes": size},
		Meta:       meta,
	})
	return nil
}
