package runsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/base/access"
	"grimoire/internal/domain/project/internal/gitsync/base/audit"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/gitprovider"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/secret"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
	"grimoire/lib/contracts/project/apispec/upsertspecs"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
)

const (
	fetchConcurrency = 4
	finTimeout       = 10 * time.Second
	genericInternal  = "terjadi kesalahan internal"
)

// GitClient diimplementasi *gitprovider.Client.
type GitClient interface {
	Policy() gitprovider.HostPolicy
	ListTree(ctx context.Context, repo gitprovider.Repo, folder string, token secret.String) ([]gitprovider.TreeEntry, error)
	FetchBlob(ctx context.Context, repo gitprovider.Repo, sha string, maxBytes int64, token secret.String) ([]byte, bool, error)
}

// Usecase mengorkestrasi urutan sync (D5).
type Usecase struct {
	access access.Checker
	repo   Repository
	git    GitClient
	bus    *bus.Registry
	audit  audit.Recorder
	clock  base.Clock
	cfg    config.Config
	log    *slog.Logger
}

func NewUsecase(a access.Checker, r Repository, g GitClient, b *bus.Registry, au audit.Recorder,
	clock base.Clock, cfg config.Config, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	if clock == nil {
		clock = base.SystemClock
	}
	return &Usecase{access: a, repo: r, git: g, bus: b, audit: au, clock: clock, cfg: cfg, log: log}
}

// fin membuat ctx untuk langkah akhir (simpan hasil, audit, unlock): nilai ctx terbawa, cancel/deadline dilepas.
func fin(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finTimeout)
}

type fetched struct {
	cand     base.Candidate
	content  []byte
	tooLarge bool
}

// Execute menjalankan sync.
func (u *Usecase) Execute(ctx context.Context, projectID string, in RunRequest, meta base.RequestMeta) (RunResponse, error) {
	started := u.clock()
	actor, err := u.access.Require(ctx, projectID)
	if err != nil {
		return RunResponse{}, err
	}
	kind, err := base.ParseKind(in.Kind)
	if err != nil {
		return RunResponse{}, err
	}
	allowed := func(p, h string) bool { return u.git.Policy().Allowed(gitprovider.Provider(p), h) }
	ref, err := base.ParseRepoURL(in.URL, in.Branch, in.Folder, allowed)
	if err != nil {
		return RunResponse{}, err
	}
	prefix := ""
	if kind == base.KindKnowledge {
		if prefix, err = base.NormalizeTargetPrefix(in.TargetPrefix); err != nil {
			return RunResponse{}, err
		}
	}

	token, err := u.token(ctx, actor, ref)
	if err != nil {
		return RunResponse{}, err
	}

	unlock, ok, err := u.repo.TryLock(ctx, projectID, kind)
	if err != nil {
		return RunResponse{}, fmt.Errorf("runsync: lock: %w", err)
	}
	if !ok {
		return RunResponse{}, base.SyncInProgress(kind)
	}
	defer func() {
		fctx, cancel := fin(ctx)
		defer cancel()
		if e := unlock(fctx); e != nil {
			u.log.WarnContext(fctx, "runsync: unlock gagal", "project_id", projectID, "kind", string(kind), "error", e)
		}
	}()

	saved, err := u.repo.UpsertConfig(ctx, base.NewConfig(projectID, kind, ref, prefix))
	if err != nil {
		return RunResponse{}, fmt.Errorf("runsync: %w", err)
	}

	run := runInfo{actor: actor, meta: meta, cfg: saved}

	cands, skipped, items, err := u.pull(ctx, kind, ref, token)
	if err != nil {
		return RunResponse{}, u.fail(ctx, run, err)
	}
	written, err := u.write(ctx, projectID, kind, prefix, actor, meta, cands, items)
	if err != nil {
		return RunResponse{}, u.fail(ctx, run, err)
	}

	report := base.NewReport(append(skipped, written...))
	fctx, cancel := fin(ctx)
	defer cancel()
	cfg, err := u.repo.SaveResult(fctx, SaveResultParams{
		ProjectID: projectID, Kind: kind, Status: string(report.Status), SyncedAt: u.clock(),
		ByID: actor.ID, ByLabel: actor.Label, Result: report.CapForStorage(base.MaxStoredReportItems),
	})
	if err != nil {
		u.log.ErrorContext(fctx, "runsync: simpan hasil gagal", "project_id", projectID, "kind", string(kind), "error", err)
		cfg = saved
	}
	u.record(fctx, run, report, "")
	u.log.InfoContext(fctx, "runsync: selesai", "project_id", projectID, "kind", string(kind), "provider", ref.Provider,
		"host", ref.Host, "status", string(report.Status), "created", report.Summary.Created,
		"updated", report.Summary.Updated, "unchanged", report.Summary.Unchanged, "skipped", report.Summary.Skipped,
		"rejected", report.Summary.Rejected, "duration_ms", u.clock().Sub(started).Milliseconds())

	dto := cfg.ToDTO()
	dto.LastResult = nil
	return RunResponse{Report: report.ToDTO(), Config: dto}, nil
}

type runInfo struct {
	actor base.Actor
	meta  base.RequestMeta
	cfg   base.Config
}

// token mengambil PAT milik pemanggil (UserID dari identity, bukan dari data tersimpan).
func (u *Usecase) token(ctx context.Context, actor base.Actor, ref base.RepoRef) (secret.String, error) {
	tok, err := bus.CallTyped[gettoken.Response](ctx, u.bus, constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN,
		gettoken.Request{UserID: actor.ID, Provider: ref.Provider, Host: ref.Host})
	if err != nil {
		return secret.String{}, fmt.Errorf("runsync: get-token: %w", err)
	}
	switch {
	case tok.Disabled:
		return secret.String{}, base.ErrFeatureDisabled
	case !tok.Found:
		return secret.String{}, base.CredentialMissing(ref.Provider, ref.Host)
	case tok.Unreadable:
		return secret.String{}, base.CredentialUnreadable(ref.Provider, ref.Host)
	}
	return tok.Token, nil
}

// pull adalah fase baca: tree, seleksi, batas, fetch. Tidak menulis ke knowledge/apispec.
func (u *Usecase) pull(ctx context.Context, kind base.Kind, ref base.RepoRef, token secret.String) ([]base.Candidate, []base.ReportItem, []fetched, error) {
	repo := gitprovider.Repo{Provider: gitprovider.Provider(ref.Provider), Host: ref.Host, Path: ref.Path, Branch: ref.Branch}
	tree, err := u.git.ListTree(ctx, repo, ref.Folder, token)
	if err != nil {
		return nil, nil, nil, err
	}
	entries := make([]base.TreeEntry, len(tree))
	for i, e := range tree {
		entries[i] = base.TreeEntry{Path: e.Path, Size: e.Size, BlobSHA: e.BlobSHA}
	}
	cands, skipped, err := base.SelectFiles(entries, ref.Folder, kind)
	if err != nil {
		if errors.Is(err, base.ErrFolderNotFound) {
			return nil, nil, nil, base.FolderNotFound(ref.Folder)
		}
		return nil, nil, nil, err
	}
	lim := base.LimitsFor(kind, u.cfg)
	if err := base.CheckBatchLimits(cands, lim); err != nil {
		return nil, nil, nil, err
	}
	items, err := u.fetchAll(ctx, repo, cands, lim, token)
	if err != nil {
		return nil, nil, nil, err
	}
	return cands, skipped, items, nil
}

// fetchAll mengambil semua blob (konkurensi 4) dengan pengecekan total kumulatif.
func (u *Usecase) fetchAll(ctx context.Context, repo gitprovider.Repo, cands []base.Candidate, lim base.Limits, token secret.String) ([]fetched, error) {
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]fetched, len(cands))
	sem := make(chan struct{}, fetchConcurrency)
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
		total    atomic.Int64
	)
	setErr := func(e error) { once.Do(func() { firstErr = e; cancel() }) }

loop:
	for i, c := range cands {
		out[i].cand = c
		if c.Size >= 0 && c.Size > lim.MaxFileBytes {
			out[i].tooLarge = true
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx2.Done():
			break loop
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx2.Err() != nil {
				return
			}
			data, tooLarge, err := u.git.FetchBlob(ctx2, repo, c.SHA, lim.MaxFileBytes, token)
			if err != nil {
				setErr(err)
				return
			}
			if tooLarge {
				out[i].tooLarge = true
				return
			}
			if e := base.CheckFetchedTotal(total.Add(int64(len(data))), lim); e != nil {
				setErr(e)
				return
			}
			out[i].content = data
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// write adalah fase tulis: satu panggilan bus. Tanpa kandidat, bus tidak dipanggil.
func (u *Usecase) write(ctx context.Context, projectID string, kind base.Kind, prefix string, actor base.Actor,
	meta base.RequestMeta, cands []base.Candidate, items []fetched) ([]base.ReportItem, error) {
	if len(cands) == 0 {
		return nil, nil
	}
	var (
		out []base.ReportItem
		err error
	)
	if kind == base.KindKnowledge {
		out, err = u.writeKnowledge(ctx, projectID, prefix, actor, meta, items)
	} else {
		out, err = u.writeSpecs(ctx, projectID, actor, meta, items)
	}
	if err != nil {
		var ee *errs.Error
		switch {
		case errors.As(err, &ee) && ee.Code == errs.CodeInvalid:
			return nil, ee
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return nil, err
		}
		return nil, &errs.Error{Code: errs.CodeInternal, Message: genericInternal, Err: err}
	}
	return out, nil
}

func (u *Usecase) writeKnowledge(ctx context.Context, projectID, prefix string, actor base.Actor, meta base.RequestMeta, items []fetched) ([]base.ReportItem, error) {
	req := upsertfiles.Request{
		ProjectID: projectID, Actor: upsertfiles.Actor{UserID: actor.ID, Role: actor.Role, Label: actor.Label},
		TargetPrefix: prefix, Source: upsertfiles.SourceGitSync, Meta: upsertfiles.Meta{IP: meta.IP, UserAgent: meta.UserAgent},
		Items: make([]upsertfiles.Item, len(items)),
	}
	for i, it := range items {
		req.Items[i] = upsertfiles.Item{Index: i, Path: it.cand.RelPath, Content: it.content, TooLarge: it.tooLarge}
	}
	res, err := bus.CallTyped[upsertfiles.Response](ctx, u.bus, constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, req)
	if err != nil {
		return nil, err
	}
	out := make([]base.ReportItem, 0, len(res.Items))
	for _, r := range res.Items {
		out = append(out, base.ReportItem{
			RepoPath: repoPathAt(items, r.Index, r.Path), Target: r.Path, Status: r.Status,
			ReasonCode: r.Reason, Issues: []base.Issue{},
		})
	}
	return out, nil
}

func (u *Usecase) writeSpecs(ctx context.Context, projectID string, actor base.Actor, meta base.RequestMeta, items []fetched) ([]base.ReportItem, error) {
	req := upsertspecs.Request{
		ProjectID: projectID, Actor: upsertspecs.Actor{UserID: actor.ID, Role: actor.Role, Label: actor.Label},
		Source: upsertspecs.SourceGitSync, Meta: upsertspecs.Meta{IP: meta.IP, UserAgent: meta.UserAgent},
		Items: make([]upsertspecs.Item, len(items)),
	}
	for i, it := range items {
		req.Items[i] = upsertspecs.Item{Index: i, SourcePath: it.cand.RepoPath, Content: it.content, TooLarge: it.tooLarge}
	}
	res, err := bus.CallTyped[upsertspecs.Response](ctx, u.bus, constants.BUS_PROJECT_APISPEC_UPSERT_SPECS, req)
	if err != nil {
		return nil, err
	}
	out := make([]base.ReportItem, 0, len(res.Items))
	for _, r := range res.Items {
		issues := make([]base.Issue, len(r.Issues))
		for i, is := range r.Issues {
			issues[i] = base.Issue{Code: is.Code, Message: is.Message, Pointer: is.Pointer, Line: is.Line}
		}
		out = append(out, base.ReportItem{
			RepoPath: repoPathAt(items, r.Index, r.SourcePath), Target: r.Name, Status: r.Status,
			ReasonCode: r.ReasonCode, Reason: r.Reason, Issues: issues,
			OpenAPIVersion: r.OpenAPIVersion, Title: r.Title, EndpointCount: r.EndpointCount,
		})
	}
	return out, nil
}

func repoPathAt(items []fetched, idx int, fallback string) string {
	if idx >= 0 && idx < len(items) {
		return items[idx].cand.RepoPath
	}
	return fallback
}

// fail memetakan error, mencatat hasil gagal dan audit, lalu mengembalikan error untuk handler.
func (u *Usecase) fail(ctx context.Context, run runInfo, err error) error {
	out, code, msg := mapError(err)
	fctx, cancel := fin(ctx)
	defer cancel()
	report := base.FailedReport(code, msg)
	if _, e := u.repo.SaveResult(fctx, SaveResultParams{
		ProjectID: run.cfg.ProjectID, Kind: run.cfg.Kind, Status: string(base.StatusFailed), SyncedAt: u.clock(),
		ByID: run.actor.ID, ByLabel: run.actor.Label, Result: report,
	}); e != nil {
		u.log.ErrorContext(fctx, "runsync: simpan hasil gagal", "project_id", run.cfg.ProjectID, "kind", string(run.cfg.Kind), "error", e)
	}
	u.record(fctx, run, report, code)
	u.log.InfoContext(fctx, "runsync: gagal", "project_id", run.cfg.ProjectID, "kind", string(run.cfg.Kind),
		"provider", run.cfg.Provider, "host", run.cfg.Host, "status", "failed", "error_code", code)
	return out
}

func (u *Usecase) record(ctx context.Context, run runInfo, report base.Report, code string) {
	u.audit.Record(ctx, audit.Event{
		Actor: run.actor, ProjectID: run.cfg.ProjectID, Kind: run.cfg.Kind, Provider: run.cfg.Provider,
		Host: run.cfg.Host, RepoPath: run.cfg.RepoPath, Branch: run.cfg.Branch, Folder: run.cfg.Folder,
		TargetPrefix: run.cfg.TargetPrefix, Status: report.Status, Summary: report.Summary, ErrorCode: code, Meta: run.meta,
	})
}

// mapError memetakan error fase baca/tulis ke error HTTP, kode, dan pesan untuk laporan.
// Pesan tidak pernah memuat token, URL berquery, atau body provider.
func mapError(err error) (out error, code, msg string) {
	var ee *errs.Error
	if errors.As(err, &ee) {
		if ee.Code == errs.CodeInternal {
			return ee, string(errs.CodeInternal), genericInternal
		}
		return ee, string(ee.Code), ee.Message
	}
	var rl *gitprovider.RateLimitedError
	switch {
	case errors.Is(err, gitprovider.ErrUnauthorized):
		ee = base.ErrGitTokenRejected
	case errors.Is(err, gitprovider.ErrForbidden):
		ee = base.ErrGitAccessDenied
	case errors.Is(err, gitprovider.ErrNotFound):
		ee = base.ErrGitRepoNotFound
	case errors.As(err, &rl):
		ee = rateLimited(int(rl.RetryAfter.Seconds()))
	case errors.Is(err, gitprovider.ErrRateLimited):
		ee = rateLimited(0)
	case errors.Is(err, gitprovider.ErrTreeTooLarge):
		ee = base.ErrGitTreeTooLarge
	case errors.Is(err, gitprovider.ErrHostNotAllowed):
		ee = errs.New(errs.CodeHostNotAllowed, base.HostNotAllowed("").Message)
	case errors.Is(err, gitprovider.ErrUpstream):
		ee = base.ErrUpstream
	case errors.Is(err, context.DeadlineExceeded):
		ee = base.ErrTimeout
	case errors.Is(err, context.Canceled):
		return context.Canceled, "CANCELED", "sync dibatalkan"
	default:
		return &errs.Error{Code: errs.CodeInternal, Message: genericInternal, Err: err}, string(errs.CodeInternal), genericInternal
	}
	return ee, string(ee.Code), ee.Message
}

func rateLimited(seconds int) *errs.Error {
	const msg = "provider Git membatasi permintaan; coba lagi nanti"
	if seconds > 0 {
		return errs.WithDetails(errs.CodeRateLimited, msg, map[string]any{"retry_after_seconds": seconds})
	}
	return errs.New(errs.CodeRateLimited, msg)
}
