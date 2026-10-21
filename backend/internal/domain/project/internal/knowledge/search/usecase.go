package search

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
)

// Repository dipenuhi oleh *store.Store.
type Repository interface {
	Search(ctx context.Context, projectID, tsq, likePrefix string, limit int) ([]base.SearchHit, int, error)
}

type Usecase struct {
	access access.Checker
	repo   Repository
}

func NewUsecase(a access.Checker, r Repository) *Usecase { return &Usecase{access: a, repo: r} }

func (u *Usecase) Execute(ctx context.Context, in Input) (base.SearchResultDTO, error) {
	if _, err := u.access.Require(ctx, in.ProjectID); err != nil {
		return base.SearchResultDTO{}, err
	}
	empty := base.SearchResultDTO{Items: []base.SearchItemDTO{}}
	tsq, ok := base.ParseQuery(in.Query)
	if !ok {
		return empty, nil
	}
	prefix, reason := base.NormalizePrefix(in.Prefix)
	if reason != "" {
		return empty, base.Invalid(reason)
	}
	n := 0
	if in.Limit != "" {
		v, err := strconv.Atoi(strings.TrimSpace(in.Limit))
		if err != nil {
			return empty, base.Invalid(base.ReasonInvalidLimit)
		}
		n = v
	}
	limit := base.ClampLimit(n, base.SearchDefaultLimit, base.SearchMaxLimit)
	hits, total, err := u.repo.Search(ctx, in.ProjectID, tsq, base.LikePrefix(prefix), limit)
	if err != nil {
		return empty, fmt.Errorf("search: query: %w", err)
	}
	res := base.NewSearchResult(hits, total)
	return res, nil
}
