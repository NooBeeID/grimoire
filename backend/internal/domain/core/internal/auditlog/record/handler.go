package record

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/audit/record"
)

type handler struct{ uc *Usecase }

// Handle adalah adapter bus untuk Call dan Publish.
func (h handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[record.Request](req)
	if err != nil {
		return nil, err
	}
	resp, err := h.uc.Execute(ctx, in)
	if err != nil {
		return record.Response{}, err
	}
	return resp, nil
}
