package searchknowledge

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/search"
)

// Searcher adalah port ke pencarian knowledge (diimplementasikan lewat bus T05).
type Searcher interface {
	Search(ctx context.Context, req search.Request) (search.Response, error)
}

type busSearcher struct{ bus *bus.Registry }

func newRepository(b *bus.Registry) Searcher { return busSearcher{bus: b} }

func (r busSearcher) Search(ctx context.Context, req search.Request) (search.Response, error) {
	return bus.CallTyped[search.Response](ctx, r.bus, constants.BUS_PROJECT_KNOWLEDGE_SEARCH, req)
}
