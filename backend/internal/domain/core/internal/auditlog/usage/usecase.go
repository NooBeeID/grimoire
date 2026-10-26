package usage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/errs"
)

const busTimeout = 5 * time.Second

// Usecase mengorkestrasi window, lookup project, daftar file, dan empat query bagian.
type Usecase struct {
	repo     Repository
	projects ProjectFinder
	files    FileLister
	now      func() time.Time
	log      *slog.Logger
}

// NewUsecase membuat Usecase.
func NewUsecase(r Repository, p ProjectFinder, f FileLister, now func() time.Time, log *slog.Logger) *Usecase {
	return &Usecase{repo: r, projects: p, files: f, now: now, log: log}
}

// Usage: q sudah tervalidasi. Tiap bagian gagal sendiri (available:false); project tak dikenal = 404.
func (u *Usecase) Usage(ctx context.Context, q base.UsageQuery) (base.UsageReport, error) {
	w := base.NewWindow(q.Range, u.now())
	if q.ProjectID != "" {
		cctx, cancel := context.WithTimeout(ctx, busTimeout)
		found, err := u.projects.Exists(cctx, q.ProjectID)
		cancel()
		if err != nil {
			return base.UsageReport{}, fmt.Errorf("usage: project: %w", err)
		}
		if !found {
			return base.UsageReport{}, errs.Wrap(errs.CodeNotFound, "project tidak ditemukan", errs.ErrNotFound)
		}
	}
	rep := base.NewUsageReport(w, q)

	if q.Wants(base.SectionTopFiles) {
		switch {
		case q.ProjectID == "":
			rep.TopFiles = base.TopFilesError(base.SectionProjectRequired)
		default:
			items, err := u.repo.TopFiles(ctx, w, q.ProjectID, base.TopFilesLimit)
			if err != nil {
				u.unavailable(ctx, base.SectionTopFiles, err)
				rep.TopFiles = base.TopFilesError(base.SectionUnavailable)
			} else {
				rep.TopFiles = base.TopFilesOK(items)
			}
		}
	}
	if q.Wants(base.SectionUnreadFiles) {
		if q.ProjectID == "" {
			rep.UnreadFiles = base.UnreadFilesError(base.SectionProjectRequired)
		} else {
			rep.UnreadFiles = u.unread(ctx, w, q.ProjectID)
		}
	}
	if q.Wants(base.SectionZeroQueries) {
		items, err := u.repo.ZeroQueries(ctx, w, q.ProjectID, base.ZeroQueriesLimit)
		if err != nil {
			u.unavailable(ctx, base.SectionZeroQueries, err)
			rep.ZeroQueries = base.ZeroQueriesError(base.SectionUnavailable)
		} else {
			rep.ZeroQueries = base.ZeroQueriesOK(items)
		}
	}
	if q.Wants(base.SectionKeys) {
		items, err := u.repo.PerKey(ctx, w, q.ProjectID, base.PerKeyLimit)
		if err != nil {
			u.unavailable(ctx, base.SectionKeys, err)
			rep.PerKey = base.PerKeyError(base.SectionUnavailable)
		} else {
			rep.PerKey = base.PerKeyOK(items)
		}
	}
	return rep, nil
}

func (u *Usecase) unread(ctx context.Context, w base.Window, projectID string) *base.UnreadFilesSection {
	cctx, cancel := context.WithTimeout(ctx, busTimeout)
	files, capped, err := u.files.ListAll(cctx, projectID)
	cancel()
	if err != nil {
		u.unavailable(ctx, base.SectionUnreadFiles, err)
		return base.UnreadFilesError(base.SectionUnavailable)
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	unread, err := u.repo.UnreadPaths(ctx, w, projectID, paths, base.UnreadLimit+1)
	if err != nil {
		u.unavailable(ctx, base.SectionUnreadFiles, err)
		return base.UnreadFilesError(base.SectionUnavailable)
	}
	return base.BuildUnread(files, unread, capped)
}

func (u *Usecase) unavailable(ctx context.Context, s base.Section, err error) {
	u.log.WarnContext(ctx, "audit usage: section unavailable", "section", string(s), "err", err)
}
