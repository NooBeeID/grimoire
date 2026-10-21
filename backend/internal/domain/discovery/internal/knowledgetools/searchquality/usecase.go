package searchquality

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/audit/scanentries"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

// Usecase mengorkestrasi paging scan audit dan akumulator kualitas (rule bisnis ada di base).
type Usecase struct {
	repo       Scanner
	now        func() time.Time
	maxEntries int
}

func NewUsecase(repo Scanner) *Usecase {
	return &Usecase{repo: repo, now: time.Now, maxEntries: base.MaxQualityScanEntries}
}

func (u *Usecase) Report(ctx context.Context, in searchquality.Request) (searchquality.Response, error) {
	p, err := ParseParams(in, u.now())
	if err != nil {
		return searchquality.Response{}, err
	}
	acc := base.NewQualityAccumulator(p.Since, p.Until, p.Window)
	var after *scanentries.Cursor
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return searchquality.Response{}, err
		}
		resp, err := u.repo.Scan(ctx, scanentries.Request{
			ProjectID: p.ProjectID,
			Actions:   []string{record.ActionMCPSearchKnowledge, record.ActionMCPReadFile},
			From:      p.Since,
			To:        p.Until.Add(p.Window),
			After:     after,
			Limit:     scanentries.MaxLimit,
		})
		if err != nil {
			return searchquality.Response{}, fmt.Errorf("searchquality: scan audit: %w", err)
		}
		total += len(resp.Entries)
		if total > u.maxEntries {
			return searchquality.Response{}, errs.Wrap(errs.CodeInvalid,
				fmt.Sprintf("rentang terlalu besar (lebih dari %d entri); persempit --since/--until", u.maxEntries), errs.ErrInvalid)
		}
		for _, e := range resp.Entries {
			acc.Add(base.EventFromAudit(e.ID, e.Action, e.ActorID, e.Target, e.Status, e.OccurredAt, e.Args, e.Result))
		}
		if resp.Next == nil {
			break
		}
		if len(resp.Entries) == 0 || (after != nil && *after == *resp.Next) {
			return searchquality.Response{}, fmt.Errorf("searchquality: scan audit: kursor tidak maju")
		}
		after = resp.Next
	}
	return searchquality.Response{Report: acc.Report(p.ProjectID)}, nil
}
