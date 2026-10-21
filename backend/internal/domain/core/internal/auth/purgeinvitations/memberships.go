package purgeinvitations

import (
	"context"
	"fmt"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/countmemberships"
)

// busCounter adalah satu-satunya bagian feature yang menyentuh bus.
type busCounter struct{ b *bus.Registry }

func (c busCounter) Count(ctx context.Context, userIDs []string) (map[string]int, error) {
	res, err := bus.CallTyped[countmemberships.Response](ctx, c.b,
		constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS,
		countmemberships.Request{UserIDs: userIDs})
	if err != nil {
		return nil, fmt.Errorf("purgeinvitations: count-memberships: %w", err)
	}
	return res.Counts, nil
}
