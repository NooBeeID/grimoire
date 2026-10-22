package busupsertspecs

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/apispec/upsertspecs"
)

// Executor adalah usecase yang dipanggil handler.
type Executor interface {
	Execute(ctx context.Context, in upsertspecs.Request) (upsertspecs.Response, error)
}

type Handler struct{ uc Executor }

func newHandler(uc Executor) *Handler { return &Handler{uc: uc} }

// Handle menerima upsertspecs.Request sebagai NILAI dan mengembalikan upsertspecs.Response sebagai nilai.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[upsertspecs.Request](req)
	if err != nil {
		return nil, err
	}
	res, err := h.uc.Execute(ctx, in)
	if err != nil {
		return nil, err
	}
	return res, nil
}
