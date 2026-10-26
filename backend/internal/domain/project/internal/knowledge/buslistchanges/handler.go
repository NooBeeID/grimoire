package buslistchanges

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

type Handler struct{ uc *Usecase }

func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[listchanges.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in)
}
