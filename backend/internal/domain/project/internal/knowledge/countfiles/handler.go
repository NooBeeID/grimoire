package countfiles

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/management/contentcount"
)

type counterUC interface {
	Count(ctx context.Context, ids []string) (map[string]int, error)
}

func handle(uc counterUC) bus.Handler {
	return func(ctx context.Context, req any) (any, error) {
		in, err := bus.Expect[contentcount.Request](req)
		if err != nil {
			return nil, err
		}
		counts, err := uc.Count(ctx, in.ProjectIDs)
		if err != nil {
			return nil, err
		}
		return contentcount.Response{Counts: counts}, nil
	}
}
