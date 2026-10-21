package inviteuser

import (
	"context"

	"grimoire/internal/infra/bus"
	contract "grimoire/lib/contracts/core/auth/inviteuser"
)

// BusHandler melayani core.auth.invite-user.
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
		Email: in.Email, Name: in.Name,
		InvitedBy: in.InvitedBy, InvitedByLabel: in.InvitedByLabel,
		ProjectName: in.ProjectName, Deliver: in.Deliver, Via: ViaProject,
	})
	if err != nil {
		return nil, err
	}
	return toContractResponse(res), nil
}

func toContractResponse(r Result) contract.Response {
	return contract.Response{
		UserID: r.User.ID, Created: r.Created, ExpiresAt: r.Invitation.ExpiresAt,
		EmailSent: r.EmailSent, EmailStatus: r.EmailStatus, Credentials: r.Credentials,
	}
}
