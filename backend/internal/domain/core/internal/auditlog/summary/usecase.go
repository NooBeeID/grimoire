package summary

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

const qualityTimeout = 5 * time.Second

// Usecase mengorkestrasi window, repository, bus T11, dan pembentukan laporan.
type Usecase struct {
	repo    Repository
	quality QualityReporter
	now     func() time.Time
	log     *slog.Logger
}

// NewUsecase membuat Usecase.
func NewUsecase(r Repository, q QualityReporter, now func() time.Time, log *slog.Logger) *Usecase {
	return &Usecase{repo: r, quality: q, now: now, log: log}
}

// Summarize: q sudah tervalidasi.
func (u *Usecase) Summarize(ctx context.Context, q base.SummaryQuery) (base.Report, error) {
	w := base.NewWindow(q.Range, u.now())
	agg, err := u.repo.Load(ctx, w, q.ProjectID)
	if err != nil {
		return base.Report{}, fmt.Errorf("summary: %w", err)
	}
	var sq *base.SearchQuality
	if q.ProjectID != "" {
		cctx, cancel := context.WithTimeout(ctx, qualityTimeout)
		rep, err := u.quality.Report(cctx, searchquality.Request{ProjectID: q.ProjectID, Since: w.From, Until: w.To})
		cancel()
		if err != nil {
			u.log.WarnContext(ctx, "audit summary: search quality unavailable", "err", err)
			sq = base.QualityUnavailable()
		} else {
			sq = base.QualityFromReport(rep)
		}
	}
	return base.BuildReport(w, q, agg, sq)
}
