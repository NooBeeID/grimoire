package countmemberships

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/management/countmemberships"
)

type repository interface {
	CountByUsers(ctx context.Context, ids []string) (map[string]int, error)
}

type Usecase struct{ repo repository }

func NewUsecase(repo repository) *Usecase { return &Usecase{repo: repo} }

// Count menghitung keanggotaan project per user (satu query, maksimal 500 id).
func (uc *Usecase) Count(ctx context.Context, req countmemberships.Request) (countmemberships.Response, error) {
	if len(req.UserIDs) > countmemberships.MaxUserIDs {
		return countmemberships.Response{}, errs.Wrap(errs.CodeInvalid, "maksimal 500 user id", errs.ErrInvalid)
	}
	ids := normalizeIDs(req.UserIDs)
	if len(ids) == 0 {
		return countmemberships.Response{Counts: map[string]int{}}, nil
	}
	counts, err := uc.repo.CountByUsers(ctx, ids)
	if err != nil {
		return countmemberships.Response{}, fmt.Errorf("countmemberships: count: %w", err)
	}
	if counts == nil {
		counts = map[string]int{}
	}
	return countmemberships.Response{Counts: counts}, nil
}

// normalizeIDs membuang id bukan uuid, mengkanonikkan, dan mendedupe (urutan kemunculan pertama).
func normalizeIDs(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		u, err := uuid.Parse(s)
		if err != nil {
			continue
		}
		k := u.String()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}
