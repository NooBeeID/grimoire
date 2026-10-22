// Package access mengotorisasi request HTTP module gitsync lewat bus project.management.check-access.
// Fail-closed: error bus -> error infra (500). Non-anggota dan project tak ada -> 404.
package access

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/checkaccess"
)

// Checker memastikan pemanggil boleh mengakses project dan mengembalikan aktornya.
type Checker interface {
	Require(ctx context.Context, projectID string) (base.Actor, error)
}

type BusChecker struct{ b *bus.Registry }

var _ Checker = (*BusChecker)(nil)

func New(b *bus.Registry) *BusChecker { return &BusChecker{b: b} }

// Require: tanpa identitas -> ErrUnauthenticated (401); error bus -> error terbungkus (500);
// project tak ada / level none / level tak dikenal -> ErrProjectNotFound (404);
// member/owner/admin -> actor.
func (c *BusChecker) Require(ctx context.Context, projectID string) (base.Actor, error) {
	id, ok := reqctx.IdentityFrom(ctx)
	actor, ok := base.ActorFrom(id, ok)
	if !ok {
		return base.Actor{}, base.ErrUnauthenticated
	}
	res, err := bus.CallTyped[checkaccess.Response](ctx, c.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
		checkaccess.Request{ProjectID: projectID, UserID: actor.ID, UserRole: actor.Role})
	if err != nil {
		return base.Actor{}, fmt.Errorf("access: check-access: %w", err)
	}
	if !res.ProjectExists {
		return base.Actor{}, base.ErrProjectNotFound
	}
	switch res.Level {
	case checkaccess.LevelMember, checkaccess.LevelOwner, checkaccess.LevelAdmin:
		return actor, nil
	default:
		return base.Actor{}, base.ErrProjectNotFound
	}
}
