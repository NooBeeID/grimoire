package listapispecs

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

// SpecLister adalah port ke provider API spec (T06).
type SpecLister interface {
	ListSpecs(ctx context.Context, req listspecs.Request) (listspecs.Response, error)
}

type busSpecLister struct{ b *bus.Registry }

func newRepository(b *bus.Registry) SpecLister { return busSpecLister{b: b} }

func (r busSpecLister) ListSpecs(ctx context.Context, req listspecs.Request) (listspecs.Response, error) {
	return bus.CallTyped[listspecs.Response](ctx, r.b, constants.BUS_PROJECT_APISPEC_LIST_SPECS, req)
}
