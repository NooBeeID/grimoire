package listfiles

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

type usecase struct{ lister Lister }

func newUsecase(l Lister) usecase { return usecase{lister: l} }

// Execute: orkestrasi saja.
func (u usecase) Execute(ctx context.Context, projectID string, p Params) (base.ListPage, error) {
	resp, err := u.lister.List(ctx, listfiles.Request{
		ProjectID: projectID,
		Prefix:    p.Prefix,
		Recursive: p.Recursive,
		Limit:     p.Limit,
		Offset:    p.Offset,
	})
	if err != nil {
		return base.ListPage{}, err
	}
	return base.NewListPage(p.Prefix, p.Recursive, p.Offset, resp), nil
}
