package countkeys

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/management/contentcount"
)

type repository interface {
	CountActiveByProjects(ctx context.Context, ids []string) (map[string]int, error)
}

type Usecase struct{ repo repository }

func NewUsecase(repo repository) *Usecase { return &Usecase{repo: repo} }

// Count menghitung key aktif per project (satu query, maksimal 100 id).
func (uc *Usecase) Count(ctx context.Context, req contentcount.Request) (contentcount.Response, error) {
	if len(req.ProjectIDs) > contentcount.MaxProjectIDs {
		return contentcount.Response{}, errs.Wrap(errs.CodeInvalid, "maksimal 100 project id", errs.ErrInvalid)
	}
	ids := normalizeIDs(req.ProjectIDs)
	if len(ids) == 0 {
		return contentcount.Response{Counts: map[string]int{}}, nil
	}
	counts, err := uc.repo.CountActiveByProjects(ctx, ids)
	if err != nil {
		return contentcount.Response{}, fmt.Errorf("countkeys: count active: %w", err)
	}
	if counts == nil {
		counts = map[string]int{}
	}
	return contentcount.Response{Counts: counts}, nil
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
