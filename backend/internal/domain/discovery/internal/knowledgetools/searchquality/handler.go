package searchquality

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

type reporter interface {
	Report(ctx context.Context, in searchquality.Request) (searchquality.Response, error)
}

type handler struct{ uc reporter }

func (h *handler) handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[searchquality.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Report(ctx, in)
}
