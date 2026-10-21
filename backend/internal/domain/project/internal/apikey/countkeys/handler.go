package countkeys

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/management/contentcount"
)

type handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

// Handle memenuhi bus.Handler.
func (h *handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[contentcount.Request](req)
	if err != nil {
		return nil, err
	}
	res, err := h.uc.Count(ctx, in)
	if err != nil {
		return nil, err
	}
	return res, nil
}
