package countfiles

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/management/contentcount"
)

type counter interface {
	CountByProject(ctx context.Context, projectIDs []string) (map[string]int, error)
}

type Usecase struct{ repo counter }

func NewUsecase(repo counter) *Usecase { return &Usecase{repo: repo} }

// Count mengembalikan jumlah file per id yang diminta. Setiap id ada di hasil (0 bila
// tanpa file atau bukan uuid). Satu query untuk seluruh batch.
func (u *Usecase) Count(ctx context.Context, ids []string) (map[string]int, error) {
	if len(ids) > contentcount.MaxProjectIDs {
		return nil, fmt.Errorf("countfiles: %d project id melebihi batas %d: %w", len(ids), contentcount.MaxProjectIDs, errs.ErrInvalid)
	}
	out := make(map[string]int, len(ids))
	byCanon := make(map[string][]string, len(ids))
	for _, id := range ids {
		out[id] = 0
		parsed, err := uuid.Parse(id)
		if err != nil {
			continue
		}
		c := parsed.String()
		if _, dup := out[id]; dup && contains(byCanon[c], id) {
			continue
		}
		byCanon[c] = append(byCanon[c], id)
	}
	if len(byCanon) == 0 {
		return out, nil
	}
	canon := make([]string, 0, len(byCanon))
	for c := range byCanon {
		canon = append(canon, c)
	}
	sort.Strings(canon)

	counts, err := u.repo.CountByProject(ctx, canon)
	if err != nil {
		return nil, fmt.Errorf("countfiles: %w", err)
	}
	for c, n := range counts {
		for _, orig := range byCanon[c] {
			out[orig] = n
		}
	}
	return out, nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
