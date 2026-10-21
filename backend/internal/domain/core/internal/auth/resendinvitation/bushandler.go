package resendinvitation

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/auth/invitation"
	contract "grimoire/lib/contracts/core/auth/resendinvitation"
)

// BusHandler melayani core.auth.resend-invitation. Bus dipercaya; otorisasi di consumer.
type BusHandler struct{ uc *Usecase }

// NewBusHandler membuat BusHandler.
func NewBusHandler(uc *Usecase) *BusHandler { return &BusHandler{uc: uc} }

// Handle mengembalikan nilai contract.Response (bukan pointer).
func (h *BusHandler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[contract.Request](req)
	if err != nil {
		return nil, err
	}
	res, err := h.uc.Execute(ctx, Input{
		UserID: in.UserID, InvitedBy: in.InvitedBy, InvitedByLabel: in.InvitedByLabel,
		ProjectName: in.ProjectName, Initial: in.Initial, Via: ViaProject,
	})
	if err != nil {
		return nil, err
	}
	out := contract.Response{EmailSent: res.EmailSent, EmailStatus: res.EmailStatus, ExpiresAt: res.ExpiresAt}
	if res.Credentials != nil {
		out.Credentials = &invitation.Credentials{
			Email: res.Credentials.Email, TemporaryPassword: res.Credentials.TemporaryPassword, LoginURL: res.Credentials.LoginURL,
		}
	}
	return out, nil
}
