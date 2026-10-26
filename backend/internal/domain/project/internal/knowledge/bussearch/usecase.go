package bussearch

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/searchrun"
	"grimoire/lib/contracts/project/knowledge/search"
)

// runner dipenuhi oleh *searchrun.Runner (orkestrasi bersama HTTP search).
type runner interface {
	Run(ctx context.Context, projectID string, terms []base.Term, likePrefix string, limit int) (searchrun.Outcome, error)
}

type Usecase struct{ runner runner }

func NewUsecase(r runner) *Usecase { return &Usecase{runner: r} }

func (u *Usecase) Execute(ctx context.Context, req search.Request) (search.Response, error) {
	if _, err := uuid.Parse(req.ProjectID); err != nil {
		return search.Response{}, base.Invalid(base.ReasonInvalidProjectID)
	}
	terms, ok := base.ParseTerms(req.Query)
	if !ok {
		return search.Response{}, nil
	}
	prefix, reason := base.NormalizePrefix(base.StripLeadingSlash(req.Prefix))
	if reason != "" {
		return search.Response{}, base.InvalidMsg(reason,
			fmt.Sprintf("prefix %q tidak valid: %s", req.Prefix, reason.Message()))
	}
	limit := base.ClampLimit(req.Limit, search.DefaultLimit, search.MaxLimit)
	out, err := u.runner.Run(ctx, req.ProjectID, terms, base.LikePrefix(prefix), limit)
	if err != nil {
		return search.Response{}, fmt.Errorf("bussearch: search: %w", err)
	}
	items := make([]search.Item, 0, len(out.Hits))
	for _, h := range out.Hits {
		items = append(items, search.Item{Path: h.Path, Score: h.Score, Snippet: base.ToMarkdownBold(h.Headline), Section: h.Section})
	}
	return search.Response{Items: items, Total: out.Total, Expanded: out.Expanded}, nil
}
