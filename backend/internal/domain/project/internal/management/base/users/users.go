// Package users membungkus lookup user lewat bus (core.auth.*) untuk module management.
package users

import (
	"context"
	"fmt"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/inviteuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
)

type Lookup interface {
	Get(ctx context.Context, userID string) (getuser.Response, error)
	FindDevelopers(ctx context.Context, q string, limit int) ([]finddevelopers.Developer, error)
	// GetByEmail memanggil get-user dengan Email (UserID kosong); Found=false bila tidak ada.
	GetByEmail(ctx context.Context, email string) (getuser.Response, error)
	// Invite memanggil core.auth.invite-user. *errs.Error provider tetap terbaca lewat errors.As.
	Invite(ctx context.Context, in inviteuser.Request) (inviteuser.Response, error)
	// ResendInvitation memanggil core.auth.resend-invitation. *errs.Error provider tetap terbaca lewat errors.As.
	ResendInvitation(ctx context.Context, in resendinvitation.Request) (resendinvitation.Response, error)
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

func (l *BusLookup) GetByEmail(ctx context.Context, email string) (getuser.Response, error) {
	res, err := bus.CallTyped[getuser.Response](ctx, l.b, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{Email: email})
	if err != nil {
		return getuser.Response{}, fmt.Errorf("users: get-user by email: %w", err)
	}
	return res, nil
}

func (l *BusLookup) Invite(ctx context.Context, in inviteuser.Request) (inviteuser.Response, error) {
	res, err := bus.CallTyped[inviteuser.Response](ctx, l.b, constants.BUS_CORE_AUTH_INVITE_USER, in)
	if err != nil {
		return inviteuser.Response{}, fmt.Errorf("users: invite-user: %w", err)
	}
	return res, nil
}

func (l *BusLookup) ResendInvitation(ctx context.Context, in resendinvitation.Request) (resendinvitation.Response, error) {
	res, err := bus.CallTyped[resendinvitation.Response](ctx, l.b, constants.BUS_CORE_AUTH_RESEND_INVITATION, in)
	if err != nil {
		return resendinvitation.Response{}, fmt.Errorf("users: resend-invitation: %w", err)
	}
	return res, nil
}
