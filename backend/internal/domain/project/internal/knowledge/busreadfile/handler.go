package busreadfile

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/knowledge/readfile"
)

type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[readfile.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in)
}
