// Package access mengotorisasi request HTTP module api-key lewat bus project.management.check-access.
// Fail-closed: error bus -> error infra (500).
package access

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/checkaccess"
)

// Checker memastikan actor boleh mengelola key project.
type Checker interface {
	Require(ctx context.Context, projectID string, actor base.Actor) error
}

type BusChecker struct{ b *bus.Registry }

var _ Checker = (*BusChecker)(nil)

func New(b *bus.Registry) *BusChecker { return &BusChecker{b: b} }

// Require: error bus -> error terbungkus; project tak ada / level none / level tak dikenal -> ErrProjectNotFound;
// member/owner/admin -> nil.
func (c *BusChecker) Require(ctx context.Context, projectID string, actor base.Actor) error {
	res, err := bus.CallTyped[checkaccess.Response](ctx, c.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
		checkaccess.Request{ProjectID: projectID, UserID: actor.ID, UserRole: actor.Role})
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
