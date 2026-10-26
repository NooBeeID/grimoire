package listrecentchanges

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apispec/listspecs"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

// FileChanges adalah port ke project.knowledge.list-changes.
type FileChanges interface {
	ListChanges(ctx context.Context, req listchanges.Request) (listchanges.Response, error)
}

// SpecLister adalah port ke project.apispec.list-specs.
type SpecLister interface {
	ListSpecs(ctx context.Context, req listspecs.Request) (listspecs.Response, error)
}

type busFiles struct{ b *bus.Registry }

func (r busFiles) ListChanges(ctx context.Context, req listchanges.Request) (listchanges.Response, error) {
	return bus.CallTyped[listchanges.Response](ctx, r.b, constants.BUS_PROJECT_KNOWLEDGE_LIST_CHANGES, req)
}

type busSpecs struct{ b *bus.Registry }

func (r busSpecs) ListSpecs(ctx context.Context, req listspecs.Request) (listspecs.Response, error) {
	return bus.CallTyped[listspecs.Response](ctx, r.b, constants.BUS_PROJECT_APISPEC_LIST_SPECS, req)
}
