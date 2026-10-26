package readsection

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	rf "grimoire/lib/contracts/project/knowledge/readfile"
	"grimoire/lib/util/mdoutline"
)

const similarMax = 5

// Usecase mengorkestrasi pembacaan satu bagian lewat bus T05 (isi utuh, Whole).
type Usecase struct{ reader Reader }

func newUsecase(r Reader) *Usecase { return &Usecase{reader: r} }

// Execute: !Found -> NotFound; bukan Markdown -> NotMarkdown; heading tak ketemu -> HeadingMiss; selain itu Section.
func (u *Usecase) Execute(ctx context.Context, projectID string, p Params) (base.ReadOutcome, error) {
	resp, err := u.reader.Read(ctx, rf.Request{ProjectID: projectID, Path: p.Path, Whole: true})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return base.NewNotFound(p.Path, resp.Similar), nil
	}
	if !base.IsMarkdown(p.Path) {
		return base.NotMarkdown{Path: p.Path}, nil
	}
	hs := mdoutline.Parse(resp.Content)
	h, count, ok := mdoutline.Find(hs, p.Heading, p.Occurrence)
	if !ok {
		miss := base.HeadingMiss{Path: p.Path, Heading: p.Heading, Occurrence: p.Occurrence, Count: count}
		if count == 0 {
			miss.Similar = base.SimilarHeadings(hs, p.Heading, similarMax)
		}
		return miss, nil
	}
	return base.NewSection(p.Path, resp.Content, h, p.Occurrence, p.Limit), nil
}
