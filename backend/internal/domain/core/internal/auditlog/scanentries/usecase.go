package scanentries

import (
	"context"
	"time"

	"grimoire/lib/contracts/core/audit/scanentries"
)

// Usecase mengorkestrasi validasi -> scan -> pemotongan halaman.
type Usecase struct {
	repo Repository
	now  func() time.Time
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, now func() time.Time) *Usecase {
	return &Usecase{repo: repo, now: now}
}

// Execute memindai entri audit project secara keyset ASC.
func (u *Usecase) Execute(ctx context.Context, in scanentries.Request) (scanentries.Response, error) {
	p, err := parse(in, u.now())
	if err != nil {
		return scanentries.Response{}, err
	}
	rows, err := u.repo.Scan(ctx, p, p.Limit+1)
	if err != nil {
		return scanentries.Response{}, err
	}
	resp := scanentries.Response{Entries: rows}
	if len(rows) > p.Limit {
		resp.Entries = rows[:p.Limit]
		last := resp.Entries[p.Limit-1]
		resp.Next = &scanentries.Cursor{OccurredAt: last.OccurredAt, ID: last.ID}
	}
	if resp.Entries == nil {
		resp.Entries = []scanentries.Entry{}
	}
	return resp, nil
}
