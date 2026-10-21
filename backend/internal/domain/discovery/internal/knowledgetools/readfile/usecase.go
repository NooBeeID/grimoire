package readfile

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	rf "grimoire/lib/contracts/project/knowledge/readfile"
)

// Usecase mengorkestrasi pembacaan file lewat bus T05.
type Usecase struct{ reader Reader }

func newUsecase(r Reader) *Usecase { return &Usecase{reader: r} }

// Execute memanggil provider lalu memilih ReadChunk atau NotFound.
func (u *Usecase) Execute(ctx context.Context, projectID string, p Params) (base.ReadOutcome, error) {
	resp, err := u.reader.Read(ctx, rf.Request{ProjectID: projectID, Path: p.Path, Offset: p.Offset, Limit: p.Limit})
	if err != nil {
		return nil, err
	}
	if resp.Found {
		return base.NewReadChunk(resp, p.Offset), nil
	}
	return base.NewNotFound(p.Path, resp.Similar), nil
}
