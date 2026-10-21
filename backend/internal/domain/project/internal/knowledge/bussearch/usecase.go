package bussearch

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/contracts/project/knowledge/search"
)

// searcher dipenuhi oleh *store.Store (satu implementasi SQL bersama HTTP search).
type searcher interface {
	Search(ctx context.Context, projectID, tsq, likePrefix string, limit int) ([]base.SearchHit, int, error)
}

type Usecase struct{ store searcher }

func NewUsecase(s searcher) *Usecase { return &Usecase{store: s} }

func (u *Usecase) Execute(ctx context.Context, req search.Request) (search.Response, error) {
	if _, err := uuid.Parse(req.ProjectID); err != nil {
		return search.Response{}, base.Invalid(base.ReasonInvalidProjectID)
	}
	tsq, ok := base.ParseQuery(req.Query)
	if !ok {
		return search.Response{}, nil
	}
	prefix, reason := base.NormalizePrefix(base.StripLeadingSlash(req.Prefix))
	if reason != "" {
		return search.Response{}, base.InvalidMsg(reason,
			fmt.Sprintf("prefix %q tidak valid: %s", req.Prefix, reason.Message()))
	}
	limit := base.ClampLimit(req.Limit, search.DefaultLimit, search.MaxLimit)
	hits, total, err := u.store.Search(ctx, req.ProjectID, tsq, base.LikePrefix(prefix), limit)
	if err != nil {
		return search.Response{}, fmt.Errorf("bussearch: search: %w", err)
	}
	items := make([]search.Item, 0, len(hits))
	for _, h := range hits {
		items = append(items, search.Item{Path: h.Path, Score: h.Score, Snippet: base.ToMarkdownBold(h.Headline)})
	}
	return search.Response{Items: items, Total: total}, nil
}
