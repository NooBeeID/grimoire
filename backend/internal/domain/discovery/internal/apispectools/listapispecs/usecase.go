package listapispecs

import (
	"context"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

type usecase struct {
	specs  SpecLister
	budget int
}

func newUsecase(s SpecLister, budget int) usecase { return usecase{specs: s, budget: budget} }

// Execute mengambil daftar spec project lalu membangun katalog.
func (u usecase) Execute(ctx context.Context, projectID string, _ Params) (base.SpecCatalog, error) {
	resp, err := u.specs.ListSpecs(ctx, listspecs.Request{ProjectID: projectID})
	if err != nil {
		return base.SpecCatalog{}, err
	}
	return base.NewSpecCatalog(resp, u.budget), nil
}
