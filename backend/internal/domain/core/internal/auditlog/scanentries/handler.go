package scanentries

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/audit/scanentries"
)

type scanner interface {
	Execute(ctx context.Context, in scanentries.Request) (scanentries.Response, error)
}

type handler struct{ uc scanner }

// Handle adalah adapter bus untuk Call.
func (h handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[scanentries.Request](req)
	if err != nil {
		return nil, err
	}
	resp, err := h.uc.Execute(ctx, in)
	if err != nil {
		return scanentries.Response{}, err
	}
	return resp, nil
}
