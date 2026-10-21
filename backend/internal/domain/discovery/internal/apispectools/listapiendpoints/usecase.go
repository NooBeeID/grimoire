package listapiendpoints

import (
	"context"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/listendpoints"
)

type usecase struct {
	lister Lister
	budget int
}

func newUsecase(l Lister, budget int) usecase { return usecase{lister: l, budget: budget} }

// Outcome: tepat satu dari Page atau Miss.
type Outcome struct {
	Page base.EndpointPage
	Miss *base.SpecMiss
}

func (o Outcome) Result() toolkit.Result {
	if o.Miss != nil {
		return o.Miss.Result()
	}
	return o.Page.Result()
}

// Execute: orkestrasi saja.
func (u usecase) Execute(ctx context.Context, projectID string, p Params) (Outcome, error) {
	resp, err := u.lister.List(ctx, listendpoints.Request{
		ProjectID: projectID,
		Spec:      p.Spec,
		Tag:       p.Tag,
		Query:     p.Query,
		Limit:     p.Limit,
		Offset:    p.Offset,
	})
	if err != nil {
		return Outcome{}, err
	}
	page, miss := base.NewEndpointPage(base.Filter{Spec: p.Spec, Tag: p.Tag, Query: p.Query}, p.Offset, resp, u.budget)
	return Outcome{Page: page, Miss: miss}, nil
}
