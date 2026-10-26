package updategroup

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/access"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
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

// Execute: access -> groupID uuid -> NormalizeTerms -> InTx{LockProject; Get; TermsInUse(except=groupID); Update}
// -> audit (setelah commit).
func (u *Usecase) Execute(ctx context.Context, projectID, groupID string, in Input, meta base.RequestMeta) (base.GroupDTO, error) {
	actor, err := u.access.Require(ctx, projectID)
	if err != nil {
		return base.GroupDTO{}, err
	}
	if !store.ValidUUID(groupID) {
		return base.GroupDTO{}, base.Invalid(base.ReasonInvalidGroupID)
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
		cur, ok, err := tx.Get(ctx, projectID, groupID)
		if err != nil {
			return err
		}
		if !ok {
			return base.ErrGroupNotFound
		}
		used, err := tx.TermsInUse(ctx, projectID, terms, groupID)
		if err != nil {
			return err
		}
		if len(used) > 0 {
			return base.TermInUse(used)
		}
		var found bool
		saved, found, err = tx.Update(ctx, cur.WithTerms(terms, actor))
		if err != nil {
			return err
		}
		if !found {
			return base.ErrGroupNotFound
		}
		return nil
	})
	if err != nil {
		var ee *errs.Error
		if errors.As(err, &ee) {
			return base.GroupDTO{}, err
		}
		return base.GroupDTO{}, fmt.Errorf("updategroup: %w", err)
	}
	u.audit.Record(ctx, audit.Event{
		Action:     record.ActionSearchSynonymUpdated,
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
