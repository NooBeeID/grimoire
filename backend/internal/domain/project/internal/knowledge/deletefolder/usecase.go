package deletefolder

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

// Execute: access -> NormalizePrefix -> root ditolak -> DeleteByPrefix -> 404 bila 0 -> audit (fail-open).
func (u *Usecase) Execute(ctx context.Context, p params, meta base.RequestMeta) (Output, error) {
	actor, err := u.access.Require(ctx, p.ProjectID)
	if err != nil {
		return Output{}, err
	}
	prefix, reason := base.NormalizePrefix(p.Path)
	if reason != "" {
		return Output{}, base.Invalid(reason)
	}
	if prefix == "" {
		return Output{}, base.Invalid(base.ReasonPrefixRequired)
	}
	n, err := u.repo.DeleteByPrefix(ctx, p.ProjectID, base.LikePrefix(prefix))
	if err != nil {
		return Output{}, fmt.Errorf("deletefolder: delete by prefix: %w", err)
	}
	if n == 0 {
		return Output{}, base.ErrFolderNotFound
	}
	u.audit.Record(ctx, audit.Event{
		Action:     record.ActionKnowledgeFolderDeleted,
		ProjectID:  p.ProjectID,
		Actor:      actor,
		TargetType: audit.TargetFolder,
		Target:     prefix,
		Args:       map[string]any{},
		Result:     map[string]any{"count": n},
		Meta:       meta,
	})
	return Output{Deleted: n}, nil
}
