package creategroup

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/access"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/lib/common/errs"
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

// Execute: access -> NormalizeTerms -> InTx{LockProject; Count; TermsInUse; Insert} -> audit (setelah commit).
func (u *Usecase) Execute(ctx context.Context, projectID string, in Input, meta base.RequestMeta) (base.GroupDTO, error) {
	actor, err := u.access.Require(ctx, projectID)
	if err != nil {
		return base.GroupDTO{}, err
	}
	terms, reason := base.NormalizeTerms(in.Terms)
	if reason != "" {
		return base.GroupDTO{}, base.Invalid(reason)
	}
	var saved base.Group
	err = u.repo.InTx(ctx, func(tx Tx) error {
		if err := tx.LockProject(ctx, projectID); err != nil {
			return err
		}
		n, err := tx.Count(ctx, projectID)
		if err != nil {
			return err
		}
		if n >= base.MaxGroupsPerProject {
			return base.GroupLimit()
		}
		used, err := tx.TermsInUse(ctx, projectID, terms, "")
		if err != nil {
			return err
		}
		if len(used) > 0 {
			return base.TermInUse(used)
		}
		saved, err = tx.Insert(ctx, base.NewGroup(projectID, terms, actor))
		return err
	})
	if err != nil {
		var ee *errs.Error
		if errors.As(err, &ee) {
			return base.GroupDTO{}, err
		}
		return base.GroupDTO{}, fmt.Errorf("creategroup: %w", err)
	}
	u.audit.Record(ctx, audit.Event{
		Action:     record.ActionSearchSynonymCreated,
		ProjectID:  projectID,
		Actor:      actor,
		TargetType: audit.TargetSynonymGroup,
		Target:     saved.ID,
		Args:       map[string]any{"terms": saved.Terms},
		Result:     map[string]any{"terms_count": len(saved.Terms)},
		Meta:       meta,
	})
	return saved.ToDTO(), nil
}
