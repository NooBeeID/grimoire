package search

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/searchrun"
)

// Runner dipenuhi oleh *searchrun.Runner.
type Runner interface {
	Run(ctx context.Context, projectID string, terms []base.Term, likePrefix string, limit int) (searchrun.Outcome, error)
}

type Usecase struct {
	access access.Checker
	runner Runner
}

func NewUsecase(a access.Checker, r Runner) *Usecase { return &Usecase{access: a, runner: r} }

func (u *Usecase) Execute(ctx context.Context, in Input) (base.SearchResultDTO, error) {
	if _, err := u.access.Require(ctx, in.ProjectID); err != nil {
		return base.SearchResultDTO{}, err
	}
	empty := base.SearchResultDTO{Items: []base.SearchItemDTO{}}
	terms, ok := base.ParseTerms(in.Query)
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
	out, err := u.runner.Run(ctx, in.ProjectID, terms, base.LikePrefix(prefix), limit)
	if err != nil {
		return empty, fmt.Errorf("search: query: %w", err)
	}
	res := base.NewSearchResult(out.Hits, out.Total, out.Expanded)
	return res, nil
}
