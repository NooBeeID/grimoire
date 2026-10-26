package readsection

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	rf "grimoire/lib/contracts/project/knowledge/readfile"
)

// Reader adalah port ke provider T05 (project.knowledge.read-file).
type Reader interface {
	Read(ctx context.Context, req rf.Request) (rf.Response, error)
}

type busReader struct{ b *bus.Registry }

func newRepository(b *bus.Registry) Reader { return busReader{b: b} }

func (r busReader) Read(ctx context.Context, req rf.Request) (rf.Response, error) {
	return bus.CallTyped[rf.Response](ctx, r.b, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, req)
}
