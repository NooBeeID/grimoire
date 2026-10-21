package getapiendpoint

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

type repository interface {
	Get(ctx context.Context, req getendpoint.Request) (getendpoint.Response, error)
}

type busRepository struct{ b *bus.Registry }

func newRepository(b *bus.Registry) repository { return busRepository{b: b} }

func (r busRepository) Get(ctx context.Context, req getendpoint.Request) (getendpoint.Response, error) {
	return bus.CallTyped[getendpoint.Response](ctx, r.b, constants.BUS_PROJECT_APISPEC_GET_ENDPOINT, req)
}
