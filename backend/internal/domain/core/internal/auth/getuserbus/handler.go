package getuserbus

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/auth/getuser"
)

type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler untuk core.auth.get-user. Mengembalikan nilai getuser.Response.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[getuser.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in.UserID)
}
