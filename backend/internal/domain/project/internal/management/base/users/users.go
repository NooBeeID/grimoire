// Package users membungkus lookup user lewat bus (core.auth.*) untuk module management.
package users

import (
	"context"
	"fmt"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
)

type Lookup interface {
	Get(ctx context.Context, userID string) (getuser.Response, error)
	FindDevelopers(ctx context.Context, q string, limit int) ([]finddevelopers.Developer, error)
}

type BusLookup struct{ b *bus.Registry }

var _ Lookup = (*BusLookup)(nil)

func New(b *bus.Registry) *BusLookup { return &BusLookup{b: b} }

func (l *BusLookup) Get(ctx context.Context, userID string) (getuser.Response, error) {
	res, err := bus.CallTyped[getuser.Response](ctx, l.b, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID: userID})
	if err != nil {
		return getuser.Response{}, fmt.Errorf("users: get-user: %w", err)
	}
	return res, nil
}

func (l *BusLookup) FindDevelopers(ctx context.Context, q string, limit int) ([]finddevelopers.Developer, error) {
	res, err := bus.CallTyped[finddevelopers.Response](ctx, l.b, constants.BUS_CORE_AUTH_FIND_DEVELOPERS,
		finddevelopers.Request{Query: q, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("users: find-developers: %w", err)
	}
	return res.Items, nil
}
