package resolvesession

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

// Handle adalah adapter bus; mengembalikan resolvesession.Response bernilai.
func Handle(uc *Usecase) bus.Handler {
	return func(ctx context.Context, req any) (any, error) {
		r, err := bus.Expect[resolvesession.Request](req)
		if err != nil {
			return nil, err
		}
		return uc.Execute(ctx, r)
	}
}
