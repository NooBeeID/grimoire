package listendpoints

import (
	"context"
	"fmt"
	"slices"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/apispec/listendpoints"
)

type endpointReader interface {
	ListSpecNames(ctx context.Context, projectID string) ([]string, error)
	QueryEndpoints(ctx context.Context, f Filter) ([]Row, int, error)
}

type Usecase struct{ repo endpointReader }

func NewUsecase(repo endpointReader) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, req listendpoints.Request) (listendpoints.Response, error) {
	if !validProjectID(req.ProjectID) {
		return listendpoints.Response{}, errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)
	}
	f := normalize(req)
	if f.Spec != "" {
		names, err := u.repo.ListSpecNames(ctx, f.ProjectID)
		if err != nil {
			return listendpoints.Response{}, fmt.Errorf("listendpoints: list spec names: %w", err)
		}
		if !slices.Contains(names, f.Spec) {
			if names == nil {
				names = []string{}
			}
			return listendpoints.Response{SpecFound: false, Specs: names, Items: []listendpoints.Endpoint{}}, nil
		}
	}
	rows, total, err := u.repo.QueryEndpoints(ctx, f)
	if err != nil {
		return listendpoints.Response{}, err
	}
	items := make([]listendpoints.Endpoint, 0, len(rows))
	for _, r := range rows {
		op := ""
		if r.OperationID != nil {
			op = *r.OperationID
		}
		tags := r.Tags
		if tags == nil {
			tags = []string{}
		}
		items = append(items, listendpoints.Endpoint{
			Spec: r.Spec, Method: r.Method, Path: r.Path, OperationID: op,
			Summary: r.Summary, Tags: tags, Deprecated: r.Deprecated,
		})
	}
	next := f.Offset + len(items)
	truncated := next < total
	if !truncated {
		next = 0
	}
	return listendpoints.Response{SpecFound: true, Items: items, Total: total, Truncated: truncated, NextOffset: next}, nil
}
