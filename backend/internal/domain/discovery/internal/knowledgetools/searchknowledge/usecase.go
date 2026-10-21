package searchknowledge

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/project/knowledge/search"
)

type usecase struct{ searcher Searcher }

func newUsecase(s Searcher) *usecase { return &usecase{searcher: s} }

// Execute memanggil pencarian untuk projectID (dari call.ProjectID) lalu membentuk entity hasil.
func (u *usecase) Execute(ctx context.Context, projectID string, p Params) (base.SearchHits, error) {
	resp, err := u.searcher.Search(ctx, search.Request{ProjectID: projectID, Query: p.Query, Prefix: p.Prefix, Limit: p.Limit})
	if err != nil {
		return base.SearchHits{}, err
	}
	return base.NewSearchHits(p.Query, p.Prefix, resp), nil
}
