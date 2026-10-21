// Package listtree menyajikan satu level pohon knowledge (folder + file) untuk sebuah prefix.
package listtree

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
)

// Repository dipenuhi oleh *store.Store.
type Repository interface {
	ListMeta(ctx context.Context, projectID, likePrefix string) ([]base.Meta, error)
}

type Usecase struct {
	access access.Checker
	repo   Repository
}

func NewUsecase(a access.Checker, r Repository) *Usecase { return &Usecase{access: a, repo: r} }

func (u *Usecase) Execute(ctx context.Context, in Input) (base.Level, error) {
	if _, err := u.access.Require(ctx, in.ProjectID); err != nil {
		return base.Level{}, err
	}
	prefix, reason := base.NormalizePrefix(in.Prefix)
	if reason != "" {
		return base.Level{}, base.Invalid(reason)
	}
	metas, err := u.repo.ListMeta(ctx, in.ProjectID, base.LikePrefix(prefix))
	if err != nil {
		return base.Level{}, fmt.Errorf("listtree: list meta: %w", err)
	}
	return base.BuildLevel(prefix, metas), nil
}
