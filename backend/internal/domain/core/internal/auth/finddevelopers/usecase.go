package finddevelopers

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"grimoire/internal/domain/core/internal/auth/base"
	contract "grimoire/lib/contracts/core/auth/finddevelopers"
)

const maxQueryRunes = 100

type developerFinder interface {
	FindActiveDevelopers(ctx context.Context, pattern string, limit int) ([]contract.Developer, error)
}

type Usecase struct{ repo developerFinder }

func NewUsecase(repo developerFinder) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, in contract.Request) (contract.Response, error) {
	q := normalizeQuery(in.Query)
	if q == "" {
		return contract.Response{Items: []contract.Developer{}}, nil
	}
	items, err := u.repo.FindActiveDevelopers(ctx, base.LikePattern(q), normalizeLimit(in.Limit))
	if err != nil {
		return contract.Response{}, fmt.Errorf("finddevelopers: find: %w", err)
	}
	if items == nil {
		items = []contract.Developer{}
	}
	return contract.Response{Items: items}, nil
}

// normalizeQuery: trim, potong ke maxQueryRunes rune, trim lagi.
func normalizeQuery(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxQueryRunes {
		s = strings.TrimSpace(string([]rune(s)[:maxQueryRunes]))
	}
	return s
}

// normalizeLimit: <=0 -> DefaultLimit; > MaxLimit -> MaxLimit.
func normalizeLimit(n int) int {
	switch {
	case n <= 0:
		return contract.DefaultLimit
	case n > contract.MaxLimit:
		return contract.MaxLimit
	}
	return n
}
