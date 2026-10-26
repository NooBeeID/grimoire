package listrecentchanges

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/project/apispec/listspecs"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

// Usecase menggabungkan perubahan file dan spec dari dua bus.
type Usecase struct {
	files FileChanges
	specs SpecLister
}

func newUsecase(f FileChanges, s SpecLister) *Usecase { return &Usecase{files: f, specs: s} }

// Execute: kind files|all memanggil FileChanges, specs|all memanggil SpecLister, lalu base.MergeChanges.
func (u *Usecase) Execute(ctx context.Context, projectID string, p Params) (base.ChangePage, error) {
	var files listchanges.Response
	var specs []listspecs.Spec
	if p.Kind == KindFiles || p.Kind == KindAll {
		res, err := u.files.ListChanges(ctx, listchanges.Request{ProjectID: projectID, Since: p.Since, Prefix: p.Prefix, Limit: p.Limit})
		if err != nil {
			return base.ChangePage{}, err
		}
		files = res
	}
	if p.Kind == KindSpecs || p.Kind == KindAll {
		res, err := u.specs.ListSpecs(ctx, listspecs.Request{ProjectID: projectID})
		if err != nil {
			return base.ChangePage{}, err
		}
		specs = res.Items
	}
	return base.MergeChanges(p.Since, p.Kind, files, specs, p.Limit), nil
}
