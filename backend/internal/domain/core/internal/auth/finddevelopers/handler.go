package finddevelopers

import (
	"context"

	"grimoire/internal/infra/bus"
	contract "grimoire/lib/contracts/core/auth/finddevelopers"
)

type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler untuk core.auth.find-developers (request dan response sebagai nilai).
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[contract.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in)
}
