package listentries

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

// Result adalah badan respons GET /api/audit-log.
type Result struct {
	Items      []base.Summary `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

// Usecase mengorkestrasi paginasi keyset.
type Usecase struct{ repo Repository }

// NewUsecase membuat Usecase.
func NewUsecase(r Repository) *Usecase { return &Usecase{repo: r} }

// List mengambil satu halaman. f harus sudah divalidasi (Limit 1..200).
func (u *Usecase) List(ctx context.Context, f base.Filter) (Result, error) {
	rows, err := u.repo.List(ctx, f, f.Limit+1)
	if err != nil {
		return Result{}, fmt.Errorf("listentries: %w", err)
	}
	var next *string
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		c := base.EncodeCursor(rows[len(rows)-1].Cursor())
		next = &c
	}
	items := make([]base.Summary, 0, len(rows))
	for _, r := range rows {
		items = append(items, r.ToSummary())
	}
	return Result{Items: items, NextCursor: next}, nil
}
