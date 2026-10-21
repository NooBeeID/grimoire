package buslistfiles

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

type Handler struct{ uc *Usecase }

func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[listfiles.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in)
}
