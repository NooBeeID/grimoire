package deletegroup

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

// Execute: access -> InTx{LockProject; Delete} -> 404 bila tidak ada -> audit (hanya bila sukses).
// groupID bukan UUID diperlakukan tidak ketemu (404), konsisten dengan store.
func (u *Usecase) Execute(ctx context.Context, projectID, groupID string, meta base.RequestMeta) error {
	actor, err := u.access.Require(ctx, projectID)
	if err != nil {
		return err
	}
	var deleted base.Group
	err = u.repo.InTx(ctx, func(tx Tx) error {
		if err := tx.LockProject(ctx, projectID); err != nil {
			return err
		}
		g, ok, err := tx.Delete(ctx, projectID, groupID)
		if err != nil {
			return err
		}
		if !ok {
			return base.ErrGroupNotFound
		}
		deleted = g
		return nil
	})
	if err != nil {
		var ee *errs.Error
		if errors.As(err, &ee) {
			return err
		}
		return fmt.Errorf("deletegroup: %w", err)
	}
	u.audit.Record(ctx, audit.Event{
		Action:     record.ActionSearchSynonymDeleted,
		ProjectID:  projectID,
		Actor:      actor,
		TargetType: audit.TargetSynonymGroup,
		Target:     deleted.ID,
		Args:       map[string]any{"terms": deleted.Terms},
		Result:     map[string]any{"terms_count": len(deleted.Terms)},
		Meta:       meta,
	})
	return nil
}
