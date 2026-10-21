// Package access mengotorisasi request HTTP module api-spec lewat bus project.management.check-access.
// Fail-closed: error bus -> error infra (500).
package access

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/checkaccess"
)

// Checker memastikan identitas di ctx anggota project (atau super_admin).
type Checker interface {
	RequireMember(ctx context.Context, projectID string) error
}

type Guard struct{ b *bus.Registry }

var _ Checker = (*Guard)(nil)

func New(b *bus.Registry) *Guard { return &Guard{b: b} }

// RequireMember: tanpa identitas -> 401; error bus -> error terbungkus (500); project tak ada, level none
// atau tak dikenal -> ErrProjectNotFound (404); member/owner/admin -> nil.
func (g *Guard) RequireMember(ctx context.Context, projectID string) error {
	id, ok := reqctx.IdentityFrom(ctx)
	if !ok || id.UserID == "" {
		return errs.New(errs.CodeUnauthorized, "perlu login")
	}
	res, err := bus.CallTyped[checkaccess.Response](ctx, g.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
		checkaccess.Request{ProjectID: projectID, UserID: id.UserID, UserRole: id.Role})
	if err != nil {
		return fmt.Errorf("access: check-access: %w", err)
	}
	if !res.ProjectExists {
		return base.ErrProjectNotFound
	}
	switch res.Level {
	case checkaccess.LevelMember, checkaccess.LevelOwner, checkaccess.LevelAdmin:
		return nil
	default:
		return base.ErrProjectNotFound
	}
}
