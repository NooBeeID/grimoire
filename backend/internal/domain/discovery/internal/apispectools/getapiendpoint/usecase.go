package getapiendpoint

import (
	"context"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

type usecase struct {
	repo     repository
	maxBytes int
}

func newUsecase(repo repository, maxBytes int) usecase {
	return usecase{repo: repo, maxBytes: maxBytes}
}

// Execute memanggil provider T06 sekali lalu memilih bentuk hasil.
func (u usecase) Execute(ctx context.Context, projectID string, p Params) (base.Lookup, error) {
	resp, err := u.repo.Get(ctx, getendpoint.Request{
		ProjectID: projectID, Spec: p.Ask.Spec, Method: p.Ask.Method, Path: p.Ask.Path,
		OperationID: p.Ask.OperationID, SchemaDepth: p.SchemaDepth, MaxBytes: u.maxBytes,
	})
	if err != nil {
		return nil, err
	}
	return base.NewLookup(p.Ask, p.SchemaDepth, u.maxBytes, resp)
}
