package busupsertfiles

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
)

type Handler struct{ uc *Usecase }

// Handle menerima upsertfiles.Request sebagai NILAI dan mengembalikan upsertfiles.Response sebagai nilai.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[upsertfiles.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in)
}
