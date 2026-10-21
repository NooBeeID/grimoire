// Package listspecs melayani GET /api/projects/{projectID}/api-specs.
package listspecs

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// specLister dipenuhi *store.Store.
type specLister interface {
	ListSpecs(ctx context.Context, projectID string) ([]base.SpecSummary, error)
}

// Result adalah hasil daftar spec; Items tidak pernah nil.
type Result struct {
	Items []base.SpecSummary
	Limit int
}

// Usecase hanya orkestrasi: aktor -> RequireMember -> ListSpecs.
type Usecase struct {
	guard    access.Checker
	specs    specLister
	maxSpecs int
}

func NewUsecase(guard access.Checker, specs specLister, maxSpecs int) *Usecase {
	return &Usecase{guard: guard, specs: specs, maxSpecs: maxSpecs}
}

func (u *Usecase) Execute(ctx context.Context, projectID string) (Result, error) {
	if _, ok := base.ActorFrom(reqctx.IdentityFrom(ctx)); !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if err := u.guard.RequireMember(ctx, projectID); err != nil {
		return Result{}, err
	}
	items, err := u.specs.ListSpecs(ctx, projectID)
	if err != nil {
		return Result{}, fmt.Errorf("listspecs: list specs: %w", err)
	}
	if items == nil {
		items = []base.SpecSummary{}
	}
	return Result{Items: items, Limit: u.maxSpecs}, nil
}
