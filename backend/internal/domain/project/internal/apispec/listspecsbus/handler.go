package listspecsbus

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler untuk project.apispec.list-specs.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[listspecs.Request](req)
	if err != nil {
		return nil, err
	}
	res, err := h.uc.Execute(ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	return res, nil
}
