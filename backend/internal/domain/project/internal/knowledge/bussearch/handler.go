package bussearch

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/knowledge/search"
)

type Handler struct{ uc *Usecase }

// Handle memenuhi bus.Handler. Isi query/snippet tidak pernah di-log.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[search.Request](req)
	if err != nil {
		return nil, err
	}
	res, err := h.uc.Execute(ctx, in)
	if err != nil {
		return nil, err
	}
	return res, nil
}
