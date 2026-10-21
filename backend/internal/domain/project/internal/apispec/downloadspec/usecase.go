// Package downloadspec melayani GET /api/projects/{projectID}/api-specs/{name}/raw.
package downloadspec

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// specFinder dipenuhi *store.Store.
type specFinder interface {
	FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
}

// RawFile adalah teks asli spec beserta metadata unduhan.
type RawFile struct {
	FileName  string
	MediaType string
	Content   string
}

type Usecase struct {
	specs specFinder
	guard access.Checker
}

func NewUsecase(specs specFinder, guard access.Checker) *Usecase {
	return &Usecase{specs: specs, guard: guard}
}

func (u *Usecase) Execute(ctx context.Context, p Params) (RawFile, error) {
	if _, ok := base.ActorFrom(reqctx.IdentityFrom(ctx)); !ok {
		return RawFile{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if err := u.guard.RequireMember(ctx, p.ProjectID); err != nil {
		return RawFile{}, err
	}
	sp, found, err := u.specs.FindSpec(ctx, p.ProjectID, p.Name)
	if err != nil {
		return RawFile{}, fmt.Errorf("downloadspec: find spec: %w", err)
	}
	if !found {
		return RawFile{}, base.ErrSpecNotFound
	}
	return RawFile{FileName: sp.FileName(), MediaType: sp.MediaType(), Content: sp.Content}, nil
}
