package getfileoutline

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	rf "grimoire/lib/contracts/project/knowledge/readfile"
)

// Usecase mengorkestrasi outline file lewat bus T05 (isi utuh, Whole).
type Usecase struct {
	reader Reader
	budget int
}

func newUsecase(r Reader, budget int) *Usecase { return &Usecase{reader: r, budget: budget} }

// Execute memanggil provider lalu memilih Outline atau NotFound.
func (u *Usecase) Execute(ctx context.Context, projectID string, p Params) (base.ReadOutcome, error) {
	resp, err := u.reader.Read(ctx, rf.Request{ProjectID: projectID, Path: p.Path, Whole: true})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return base.NewNotFound(p.Path, resp.Similar), nil
	}
	return base.NewOutline(p.Path, resp, u.budget), nil
}
