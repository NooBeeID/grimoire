package createadmin

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/auth/createadmin"
)

type handler struct{ uc *Usecase }

// handle adalah adaptor bus. Request berisi password, jadi tidak pernah di-log.
func (h *handler) handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[createadmin.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Create(ctx, in)
}
