package backfillsections

import (
	"context"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

const (
	batchSize  = 200
	maxBatches = 10
)

type repository interface {
	NullSections(ctx context.Context, limit int) ([]Pending, error)
	SetSections(ctx context.Context, id string, sections []byte) error
}

type Usecase struct{ repo repository }

func NewUsecase(repo repository) *Usecase { return &Usecase{repo: repo} }

// Run mengisi sections untuk baris lama, maksimal maxBatches x batchSize per panggilan.
// Berhenti saat batch kosong atau ctx selesai. Idempoten.
func (u *Usecase) Run(ctx context.Context) error {
	for range maxBatches {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := u.repo.NullSections(ctx, batchSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, p := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := u.repo.SetSections(ctx, p.ID, base.SectionsJSON(p.Content)); err != nil {
				return err
			}
		}
	}
	return nil
}
