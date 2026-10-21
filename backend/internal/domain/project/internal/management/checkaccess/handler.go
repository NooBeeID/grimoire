package checkaccess

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/management/checkaccess"
)

type handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

// Handle adalah bus.Handler; Response dikembalikan sebagai nilai.
func (h *handler) Handle(ctx context.Context, req any) (any, error) {
	r, err := bus.Expect[checkaccess.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, r)
}
