package listapiendpoints

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apispec/listendpoints"
)

// Lister adalah port ke provider API spec (T06). Satu-satunya tempat bus dipanggil.
type Lister interface {
	List(ctx context.Context, req listendpoints.Request) (listendpoints.Response, error)
}

type busLister struct{ b *bus.Registry }

func newRepository(b *bus.Registry) Lister { return busLister{b: b} }

func (r busLister) List(ctx context.Context, req listendpoints.Request) (listendpoints.Response, error) {
	return bus.CallTyped[listendpoints.Response](ctx, r.b, constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, req)
}
