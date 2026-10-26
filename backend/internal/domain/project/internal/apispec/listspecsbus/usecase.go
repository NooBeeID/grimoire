// Package listspecsbus menyediakan handler bus project.apispec.list-specs (provider T06, consumer T09).
package listspecsbus

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

// specLister dipenuhi *store.Store.
type specLister interface {
	ListSpecs(ctx context.Context, projectID string) ([]base.SpecSummary, error)
}

type Usecase struct{ specs specLister }

func NewUsecase(specs specLister) *Usecase { return &Usecase{specs: specs} }

// Execute mengembalikan spec milik projectID saja (store memfilter project_id).
func (u *Usecase) Execute(ctx context.Context, projectID string) (listspecs.Response, error) {
	if _, err := uuid.Parse(projectID); err != nil {
		return listspecs.Response{}, errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)
	}
	rows, err := u.specs.ListSpecs(ctx, projectID)
	if err != nil {
		return listspecs.Response{}, fmt.Errorf("listspecsbus: list specs: %w", err)
	}
	items := make([]listspecs.Spec, 0, len(rows))
	for _, s := range rows {
		items = append(items, listspecs.Spec{
			Name:           s.Name,
			Title:          s.Title,
			ApiVersion:     s.APIVersion,
			OpenapiVersion: s.OpenAPIVersion,
			EndpointCount:  s.EndpointCount,
			UpdatedAt:      s.UpdatedAt.UTC(),
			CreatedAt:      s.CreatedAt.UTC(),
			SizeBytes:      s.SizeBytes,
		})
	}
	return listspecs.Response{Items: items}, nil
}
