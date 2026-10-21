package purgeinvitations

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
)

const (
	batchSize  = 500 // harus <= countmemberships.MaxUserIDs (dijaga tes)
	maxBatches = 20
)

type repository interface {
	ListCandidates(ctx context.Context, cutoff time.Time, after *Candidate, limit int) ([]Candidate, error)
	DeleteUsers(ctx context.Context, ids []string, cutoff time.Time) (int64, error)
}

type membershipCounter interface {
	Count(ctx context.Context, userIDs []string) (map[string]int, error)
}

type Usecase struct {
	repo       repository
	counter    membershipCounter
	clock      base.Clock
	purgeAfter time.Duration
	log        *slog.Logger
}

// New panic bila purgeAfter tidak positif (kesalahan wiring).
func New(repo repository, counter membershipCounter, clock base.Clock, purgeAfter time.Duration, log *slog.Logger) *Usecase {
	if purgeAfter <= 0 {
		panic("purgeinvitations: purgeAfter harus > 0")
	}
	return &Usecase{repo: repo, counter: counter, clock: clock, purgeAfter: purgeAfter, log: log}
}

// Run menghapus akun undangan yang kedaluwarsa lewat batas purge dan tanpa keanggotaan project.
// Keanggotaan ditanyakan lewat bus; bila bus gagal, batch itu tidak dihapus (fail-closed) dan
// Run mengembalikan error. Race yang diterima: anggota ditambahkan tepat antara cek count dan
// DELETE tidak ditutup dengan lock lintas module. DELETE mengecek ulang syarat undangan
// menunggu dan lewat batas, sehingga akun yang diterima/diundang ulang tidak terhapus.
// Idempoten.
func (u *Usecase) Run(ctx context.Context) error {
	wall := time.Now()
	start := u.clock()
	cutoff := start.Add(-u.purgeAfter)

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("purgeinvitations: %w", err)
	}

	var (
		cursor                        *Candidate
		candidates, retained, deleted int
		batches                       int
	)
	for batches < maxBatches {
		cands, err := u.repo.ListCandidates(ctx, cutoff, cursor, batchSize)
		if err != nil {
			u.logPartial(ctx, deleted)
			return fmt.Errorf("purgeinvitations: %w", err)
		}
		if len(cands) == 0 {
			break
		}
		batches++
		candidates += len(cands)

		ids := make([]string, len(cands))
		for i, c := range cands {
			ids[i] = c.UserID
		}
		counts, err := u.counter.Count(ctx, ids)
		if err != nil {
			u.log.ErrorContext(ctx, "auth purge invitations dibatalkan: count-memberships gagal",
				"deleted", deleted, "err", err)
			return fmt.Errorf("purgeinvitations: %w", err)
		}
		deletable := make([]string, 0, len(ids))
		for _, id := range ids {
			if counts[id] > 0 {
				retained++
				continue
			}
			deletable = append(deletable, id)
		}
		if len(deletable) > 0 {
			n, err := u.repo.DeleteUsers(ctx, deletable, cutoff)
			if err != nil {
				u.logPartial(ctx, deleted)
				return fmt.Errorf("purgeinvitations: %w", err)
			}
			deleted += int(n)
		}

		last := cands[len(cands)-1]
		cursor = &last
		if len(cands) < batchSize {
			break
		}
	}
	u.log.InfoContext(ctx, "auth purge invitations selesai",
		"candidates", candidates, "retained", retained, "deleted", deleted,
		"batches", batches, "duration_ms", time.Since(wall).Milliseconds())
	return nil
}

func (u *Usecase) logPartial(ctx context.Context, deleted int) {
	u.log.InfoContext(ctx, "auth purge invitations sebagian", "deleted", deleted)
}
