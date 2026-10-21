package getendpoint

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler untuk project.apispec.get-endpoint. Response dikembalikan sebagai nilai.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	r, err := bus.Expect[getendpoint.Request](req)
	if err != nil {
		return nil, err
	}
	res, err := h.uc.Execute(ctx, r)
	if err != nil {
		return nil, err
	}
	return res, nil
}
