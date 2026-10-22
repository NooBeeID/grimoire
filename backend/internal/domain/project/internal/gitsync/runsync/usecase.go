package runsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
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

// GitClient membuat Session ber-pacing per sync (diimplementasi adapter di atas *gitprovider.Client).
type GitClient interface {
	Policy() gitprovider.HostPolicy
	Session(onWait func(gitprovider.Wait)) GitSession
}

// GitSession adalah jalur request ber-pacing untuk satu sync; aman dipakai bersamaan.
type GitSession interface {
	ListTree(ctx context.Context, repo gitprovider.Repo, folder string, token secret.String) ([]gitprovider.TreeEntry, error)
	FetchBlob(ctx context.Context, repo gitprovider.Repo, sha string, maxBytes int64, token secret.String) ([]byte, bool, error)
	Stats() gitprovider.Stats
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

// Run adalah sync yang sudah lolos validasi dan memegang lock; wajib diakhiri Execute atau Abort.
type Run struct {
	uc      *Usecase
	kind    base.Kind
	ref     base.RepoRef
	prefix  string
	token   secret.String
	info    runInfo
	started time.Time
	unlock  func(context.Context) error
	once    sync.Once
	running atomic.Bool
}

// Start mengerjakan akses, parse, token, lock, dan simpan config. Error di sini selalu dikembalikan
// sebelum ada stream. Sukses: lock dipegang Run.
func (u *Usecase) Start(ctx context.Context, projectID string, in RunRequest, meta base.RequestMeta) (*Run, error) {
	started := u.clock()
	actor, err := u.access.Require(ctx, projectID)
	if err != nil {
		return nil, err
	}
	kind, err := base.ParseKind(in.Kind)
	if err != nil {
		return nil, err
	}
	allowed := func(p, h string) bool { return u.git.Policy().Allowed(gitprovider.Provider(p), h) }
	ref, err := base.ParseRepoURL(in.URL, in.Branch, in.Folder, allowed)
	if err != nil {
		return nil, err
	}
	prefix := ""
	if kind == base.KindKnowledge {
		if prefix, err = base.NormalizeTargetPrefix(in.TargetPrefix); err != nil {
			return nil, err
		}
	}

	token, err := u.token(ctx, actor, ref)
	if err != nil {
		return nil, err
	}

	unlock, ok, err := u.repo.TryLock(ctx, projectID, kind)
	if err != nil {
		return nil, fmt.Errorf("runsync: lock: %w", err)
	}
	if !ok {
		return nil, base.SyncInProgress(kind)
	}
	r := &Run{uc: u, kind: kind, ref: ref, prefix: prefix, token: token, started: started, unlock: unlock}

	saved, err := u.repo.UpsertConfig(ctx, base.NewConfig(projectID, kind, ref, prefix))
	if err != nil {
		r.release(ctx)
		return nil, fmt.Errorf("runsync: %w", err)
	}
	r.info = runInfo{actor: actor, meta: meta, cfg: saved}
	return r, nil
}

// release melepas lock tepat sekali.
func (r *Run) release(ctx context.Context) {
	r.once.Do(func() {
		fctx, cancel := fin(ctx)
		defer cancel()
		if e := r.unlock(fctx); e != nil {
			r.uc.log.WarnContext(fctx, "runsync: unlock gagal", "project_id", r.info.cfg.ProjectID, "kind", string(r.kind), "error", e)
		}
	})
}

// Abort melepas lock bila Execute tidak akan dipanggil. No-op bila Execute sudah dimulai.
func (r *Run) Abort(ctx context.Context) {
	if r.running.Load() {
		return
	}
	r.release(ctx)
}

// Execute menjalankan pull, write, simpan hasil, dan audit. Hanya boleh dipanggil sekali.
func (r *Run) Execute(ctx context.Context, sink Sink) (RunResponse, error) {
	if !r.running.CompareAndSwap(false, true) {
		return RunResponse{}, &errs.Error{Code: errs.CodeInternal, Message: genericInternal, Err: errors.New("runsync: Execute dipanggil lebih dari sekali")}
	}
	defer r.release(ctx)
	if sink == nil {
		sink = NopSink{}
	}
	u := r.uc
	sess := u.git.Session(func(w gitprovider.Wait) {
		sink.Waiting(int(math.Ceil(w.Duration.Seconds())), string(w.Reason))
	})
	cands, skipped, items, err := u.pull(ctx, sess, r, sink)
	if err != nil {
		return RunResponse{}, u.fail(ctx, r.withStats(sess), err)
	}
	written, err := u.write(ctx, r, cands, items, sink)
	if err != nil {
		return RunResponse{}, u.fail(ctx, r.withStats(sess), err)
	}

	report := base.NewReport(append(skipped, written...))
	fctx, cancel := fin(ctx)
	defer cancel()
	info := r.withStats(sess)
	cfg, err := u.repo.SaveResult(fctx, SaveResultParams{
		ProjectID: info.cfg.ProjectID, Kind: r.kind, Status: string(report.Status), SyncedAt: u.clock(),
		ByID: info.actor.ID, ByLabel: info.actor.Label, Result: report.CapForStorage(base.MaxStoredReportItems),
	})
	if err != nil {
		u.log.ErrorContext(fctx, "runsync: simpan hasil gagal", "project_id", info.cfg.ProjectID, "kind", string(r.kind), "error", err)
		cfg = info.cfg
	}
	u.record(fctx, info, report, "")
	u.log.InfoContext(fctx, "runsync: selesai", "project_id", info.cfg.ProjectID, "kind", string(r.kind), "provider", r.ref.Provider,
		"host", r.ref.Host, "status", string(report.Status), "created", report.Summary.Created,
		"updated", report.Summary.Updated, "unchanged", report.Summary.Unchanged, "skipped", report.Summary.Skipped,
		"rejected", report.Summary.Rejected, "wait_count", info.stats.Waits, "wait_total_ms", info.stats.TotalWait.Milliseconds(),
		"duration_ms", u.clock().Sub(r.started).Milliseconds())

	dto := cfg.ToDTO()
	dto.LastResult = nil
	return RunResponse{Report: report.ToDTO(), Config: dto}, nil
}

func (r *Run) withStats(sess GitSession) runInfo {
	info := r.info
	info.stats = sess.Stats()
	return info
}

type runInfo struct {
	actor base.Actor
	meta  base.RequestMeta
	cfg   base.Config
	stats gitprovider.Stats
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
func (u *Usecase) pull(ctx context.Context, sess GitSession, r *Run, sink Sink) ([]base.Candidate, []base.ReportItem, []fetched, error) {
	ref := r.ref
	repo := gitprovider.Repo{Provider: gitprovider.Provider(ref.Provider), Host: ref.Host, Path: ref.Path, Branch: ref.Branch}
	tree, err := sess.ListTree(ctx, repo, ref.Folder, r.token)
	if err != nil {
		return nil, nil, nil, err
	}
	entries := make([]base.TreeEntry, len(tree))
	for i, e := range tree {
		entries[i] = base.TreeEntry{Path: e.Path, Size: e.Size, BlobSHA: e.BlobSHA}
	}
	cands, skipped, err := base.SelectFiles(entries, ref.Folder, r.kind)
	if err != nil {
		if errors.Is(err, base.ErrFolderNotFound) {
			return nil, nil, nil, base.FolderNotFound(ref.Folder)
		}
		return nil, nil, nil, err
	}
	lim := base.LimitsFor(r.kind, u.cfg)
	if err := base.CheckBatchLimits(cands, lim); err != nil {
		return nil, nil, nil, err
	}
	sink.Start(len(cands))
	items, err := u.fetchAll(ctx, sess, repo, cands, lim, r.token, sink)
	if err != nil {
		return nil, nil, nil, err
	}
	return cands, skipped, items, nil
}

// progressEmitter menaikkan counter dan memanggil sink di bawah satu mutex agar done monoton.
type progressEmitter struct {
	mu    sync.Mutex
	n     int
	total int
	sink  Sink
}

func (p *progressEmitter) done(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	p.sink.Progress(p.n, p.total, path)
}

// fetchAll mengambil semua blob (konkurensi 4) dengan pengecekan total kumulatif.
func (u *Usecase) fetchAll(ctx context.Context, sess GitSession, repo gitprovider.Repo, cands []base.Candidate, lim base.Limits, token secret.String, sink Sink) ([]fetched, error) {
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	emitter := &progressEmitter{total: len(cands), sink: sink}
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
			emitter.done(c.RepoPath)
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
			data, tooLarge, err := sess.FetchBlob(ctx2, repo, c.SHA, lim.MaxFileBytes, token)
			if err != nil {
				setErr(err)
				return
			}
			if tooLarge {
				out[i].tooLarge = true
				emitter.done(c.RepoPath)
				return
			}
			if e := base.CheckFetchedTotal(total.Add(int64(len(data))), lim); e != nil {
				setErr(e)
				return
			}
			out[i].content = data
			emitter.done(c.RepoPath)
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
func (u *Usecase) write(ctx context.Context, r *Run, cands []base.Candidate, items []fetched, sink Sink) ([]base.ReportItem, error) {
	if len(cands) == 0 {
		return nil, nil
	}
	projectID, kind, prefix, actor, meta := r.info.cfg.ProjectID, r.kind, r.prefix, r.info.actor, r.info.meta
	sink.Saving()
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
		WaitCount: run.stats.Waits, WaitTotal: run.stats.TotalWait,
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
		ee = rateLimited(int(math.Ceil(rl.RetryAfter.Seconds())))
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
