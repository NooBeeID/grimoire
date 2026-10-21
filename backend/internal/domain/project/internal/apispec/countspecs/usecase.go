package countspecs

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/management/contentcount"
)

type counter interface {
	CountByProjects(ctx context.Context, ids []string) (map[string]int, error)
}

type Usecase struct{ repo counter }

func NewUsecase(repo counter) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, in contentcount.Request) (contentcount.Response, error) {
	if len(in.ProjectIDs) > contentcount.MaxProjectIDs {
		return contentcount.Response{}, errs.Wrap(errs.CodeInvalid, "project_ids melebihi batas", errs.ErrInvalid)
	}
	ids := normalizeIDs(in.ProjectIDs)
	if len(ids) == 0 {
		return contentcount.Response{Counts: map[string]int{}}, nil
	}
	counts, err := u.repo.CountByProjects(ctx, ids)
	if err != nil {
		return contentcount.Response{}, fmt.Errorf("countspecs: count by projects: %w", err)
	}
	if counts == nil {
		counts = map[string]int{}
	}
	return contentcount.Response{Counts: counts}, nil
}

// normalizeIDs membuang id bukan uuid dan duplikat; hasil kanonik huruf kecil, urutan pertama-muncul.
func normalizeIDs(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		p, err := uuid.Parse(s)
		if err != nil {
			continue
		}
		c := p.String()
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}
