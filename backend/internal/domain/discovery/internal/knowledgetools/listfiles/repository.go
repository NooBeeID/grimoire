package listfiles

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

// Lister adalah port ke provider knowledge (T05). Satu-satunya tempat bus dipanggil.
type Lister interface {
	List(ctx context.Context, req listfiles.Request) (listfiles.Response, error)
}

type busLister struct{ b *bus.Registry }

func newRepository(b *bus.Registry) Lister { return busLister{b: b} }

func (r busLister) List(ctx context.Context, req listfiles.Request) (listfiles.Response, error) {
	return bus.CallTyped[listfiles.Response](ctx, r.b, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, req)
}
