package runsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/base/audit"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/gitprovider"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/common/secret"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
	"grimoire/lib/contracts/project/apispec/upsertspecs"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
)

const canary = "ghp_CANARY0123456789abcdefCANARY"

type calls struct {
	mu  sync.Mutex
	seq []string
}

func (c *calls) add(s string) { c.mu.Lock(); c.seq = append(c.seq, s); c.mu.Unlock() }
func (c *calls) list() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.seq...)
}
func (c *calls) count(prefix string) int {
	n := 0
	for _, s := range c.list() {
		if strings.HasPrefix(s, prefix) {
			n++
		}
	}
	return n
}

type fakeAccess struct {
	actor base.Actor
	err   error
	c     *calls
}

func (f *fakeAccess) Require(context.Context, string) (base.Actor, error) {
	f.c.add("access")
	if f.err != nil {
		return base.Actor{}, f.err
	}
	return f.actor, nil
}

type fakeRepo struct {
	c        *calls
	lockOK   bool
	lockErr  error
	unlockEr error
	saved    []SaveResultParams
	upserted []base.Config
	saveErr  error
	ctxErrs  []error
}

func (f *fakeRepo) TryLock(context.Context, string, base.Kind) (func(context.Context) error, bool, error) {
	f.c.add("lock")
	if f.lockErr != nil {
		return nil, false, f.lockErr
	}
	if !f.lockOK {
		return nil, false, nil
	}
	return func(ctx context.Context) error {
		f.c.add("unlock")
		f.ctxErrs = append(f.ctxErrs, ctx.Err())
		return f.unlockEr
	}, true, nil
}
func (f *fakeRepo) UpsertConfig(_ context.Context, cfg base.Config) (base.Config, error) {
	f.c.add("upsert")
	f.upserted = append(f.upserted, cfg)
	return cfg, nil
}
func (f *fakeRepo) SaveResult(ctx context.Context, p SaveResultParams) (base.Config, error) {
	f.c.add("save")
	f.ctxErrs = append(f.ctxErrs, ctx.Err())
	f.saved = append(f.saved, p)
	if f.saveErr != nil {
		return base.Config{}, f.saveErr
	}
	st := p.Status
	return base.Config{ProjectID: p.ProjectID, Kind: p.Kind, LastStatus: &st, TargetPrefix: "guides"}, nil
}

type fakeAudit struct {
	c      *calls
	events []audit.Event
}

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) {
	f.c.add("audit")
	f.events = append(f.events, ev)
}

type fakeGit struct {
	c          *calls
	tree       []gitprovider.TreeEntry
	treeErr    error
	blobs      map[string][]byte
	blobErr    error
	tooLarge   map[string]bool
	delay      time.Duration
	cur, peak  atomic.Int32
	tokens     []string
	allowedGL  []string
	blobCalled atomic.Int32
	// onTree dipanggil di ListTree dengan onWait sesi (untuk mensimulasikan tunggu rate limit).
	onTree func(onWait func(gitprovider.Wait))
	onWait func(gitprovider.Wait)
	// blobGate (bila non-nil) menahan FetchBlob sampai ditutup.
	blobGate chan struct{}
	stats    gitprovider.Stats
}

func (f *fakeGit) Session(onWait func(gitprovider.Wait)) GitSession {
	f.onWait = onWait
	return f
}
func (f *fakeGit) Stats() gitprovider.Stats { return f.stats }

// Execute hanya untuk tes: Start lalu Run.Execute dengan NopSink.
func (u *Usecase) Execute(ctx context.Context, projectID string, in RunRequest, meta base.RequestMeta) (RunResponse, error) {
	run, err := u.Start(ctx, projectID, in, meta)
	if err != nil {
		return RunResponse{}, err
	}
	return run.Execute(ctx, NopSink{})
}

func (f *fakeGit) Policy() gitprovider.HostPolicy {
	return gitprovider.HostPolicy{AllowedGitLabHosts: f.allowedGL}
}
func (f *fakeGit) ListTree(_ context.Context, _ gitprovider.Repo, _ string, tok secret.String) ([]gitprovider.TreeEntry, error) {
	f.c.add("tree")
	f.tokens = append(f.tokens, tok.Reveal())
	if f.onTree != nil {
		f.onTree(f.onWait)
	}
	return f.tree, f.treeErr
}
func (f *fakeGit) FetchBlob(ctx context.Context, _ gitprovider.Repo, sha string, _ int64, tok secret.String) ([]byte, bool, error) {
	f.blobCalled.Add(1)
	if f.blobGate != nil {
		select {
		case <-f.blobGate:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
	n := f.cur.Add(1)
	defer f.cur.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
	if f.blobErr != nil {
		return nil, false, f.blobErr
	}
	return f.blobs[sha], f.tooLarge[sha], nil
}

type env struct {
	uc        *Usecase
	c         *calls
	repo      *fakeRepo
	git       *fakeGit
	aud       *fakeAudit
	acc       *fakeAccess
	log       *bytes.Buffer
	tokReq    []gettoken.Request
	upF       []upsertfiles.Request
	upS       []upsertspecs.Request
	tokResp   gettoken.Response
	filesResp func(upsertfiles.Request) (upsertfiles.Response, error)
	specsResp func(upsertspecs.Request) (upsertspecs.Response, error)
}

func newEnv(t *testing.T) *env {
	t.Helper()
	c := &calls{}
	e := &env{c: c, log: &bytes.Buffer{}}
	e.repo = &fakeRepo{c: c, lockOK: true}
	e.git = &fakeGit{c: c, blobs: map[string][]byte{}, tooLarge: map[string]bool{}}
	e.aud = &fakeAudit{c: c}
	e.acc = &fakeAccess{c: c, actor: base.Actor{ID: "u1", Role: "developer", Label: "Rina"}}
	e.tokResp = gettoken.Response{Found: true, Token: secret.New(canary)}
	log := slog.New(slog.NewJSONHandler(e.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	b := bus.NewRegistry(log)
	b.Register(constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, func(_ context.Context, req any) (any, error) {
		r, err := bus.Expect[gettoken.Request](req)
		if err != nil {
			return nil, err
		}
		c.add("token")
		e.tokReq = append(e.tokReq, r)
		return e.tokResp, nil
	})
	b.Register(constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, func(_ context.Context, req any) (any, error) {
		r, err := bus.Expect[upsertfiles.Request](req)
		if err != nil {
			return nil, err
		}
		c.add("upsert-files")
		e.upF = append(e.upF, r)
		if e.filesResp != nil {
			return e.filesResp(r)
		}
		res := upsertfiles.Response{}
		for _, it := range r.Items {
			st := "created"
			reason := ""
			if it.TooLarge {
				st, reason = "rejected", "too_large"
			}
			res.Items = append(res.Items, upsertfiles.ItemResult{Index: it.Index, Path: joinPrefix(r.TargetPrefix, it.Path), Status: st, Reason: reason})
		}
		return res, nil
	})
	b.Register(constants.BUS_PROJECT_APISPEC_UPSERT_SPECS, func(_ context.Context, req any) (any, error) {
		r, err := bus.Expect[upsertspecs.Request](req)
		if err != nil {
			return nil, err
		}
		c.add("upsert-specs")
		e.upS = append(e.upS, r)
		if e.specsResp != nil {
			return e.specsResp(r)
		}
		res := upsertspecs.Response{}
		for _, it := range r.Items {
			res.Items = append(res.Items, upsertspecs.ItemResult{Index: it.Index, SourcePath: it.SourcePath, Name: "n" + fmt.Sprint(it.Index), Status: "created", EndpointCount: 3})
		}
		return res, nil
	})
	clock := func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }
	cfg := config.Config{MaxFileBytes: 1000, MaxUploadFiles: 50, MaxUploadBytes: 10000, APISpecMaxFileBytes: 1000, APISpecMaxSpecs: 50}
	e.uc = NewUsecase(e.acc, e.repo, e.git, b, e.aud, clock, cfg, log)
	return e
}

func joinPrefix(p, s string) string {
	if p == "" {
		return s
	}
	return p + "/" + s
}

func (e *env) addFile(path, content string) {
	sha := "sha-" + path
	e.git.tree = append(e.git.tree, gitprovider.TreeEntry{Path: path, Size: int64(len(content)), BlobSHA: sha})
	e.git.blobs[sha] = []byte(content)
}

func ctxWithID() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: "developer", Name: "Rina"})
}

func kreq() RunRequest {
	return RunRequest{Kind: "knowledge", URL: "https://github.com/noobee/grimoire-docs/tree/main/docs", TargetPrefix: "guides"}
}

func TestKnowledgeSuccess(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/auth/login-flow.md", "a")
	e.addFile("docs/api/rate-limit.md", "b")
	e.addFile("docs/img/diagram.png", "c")
	res, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"access", "token", "lock", "upsert", "tree", "upsert-files", "save", "audit", "unlock"}
	if got := e.c.list(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("urutan %v", got)
	}
	r := e.upF[0]
	if r.TargetPrefix != "guides" || r.Source != "git_sync" || r.Actor.UserID != "u1" || r.Meta.IP != "1.2.3.4" || len(r.Items) != 2 ||
		r.Items[0].Path != "api/rate-limit.md" || r.Items[1].Path != "auth/login-flow.md" {
		t.Fatalf("%+v", r)
	}
	if res.Report.Status != "success" || res.Report.Summary.Created != 2 || res.Report.Summary.Skipped != 1 {
		t.Fatalf("%+v", res.Report)
	}
	var skipped bool
	for _, it := range res.Report.Items {
		if it.RepoPath == "docs/img/diagram.png" && it.Status == "skipped" && it.ReasonCode == "unsupported_type" {
			skipped = true
		}
	}
	if !skipped || res.Config.LastResult != nil {
		t.Fatalf("%+v", res)
	}
	s := e.repo.saved[0]
	if s.Status != "success" || s.ByID != "u1" || s.ByLabel != "Rina" || e.repo.upserted[0].TargetPrefix != "guides" {
		t.Fatalf("%+v", s)
	}
	ev := e.aud.events[0]
	if ev.Kind != base.KindKnowledge || ev.RepoPath != "noobee/grimoire-docs" || ev.Branch != "main" || ev.Folder != "docs" || ev.ErrorCode != "" || ev.Summary.Created != 2 {
		t.Fatalf("%+v", ev)
	}
}

func TestTokenIsActorsAndNoWriteOnCredentialErrors(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	if _, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if e.tokReq[0].UserID != "u1" || e.tokReq[0].Provider != "github" || e.tokReq[0].Host != "github.com" || e.git.tokens[0] != canary {
		t.Fatalf("%+v %v", e.tokReq, e.git.tokens)
	}
	for name, tc := range map[string]struct {
		resp gettoken.Response
		code errs.Code
	}{
		"disabled":   {gettoken.Response{Disabled: true}, errs.CodeGitFeatureDisabled},
		"missing":    {gettoken.Response{}, base.CodeCredentialMissing},
		"unreadable": {gettoken.Response{Found: true, Unreadable: true}, base.CodeCredentialUnreadable},
	} {
		e := newEnv(t)
		e.tokResp = tc.resp
		_, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
		var ee *errs.Error
		if !errors.As(err, &ee) || ee.Code != tc.code {
			t.Fatalf("%s: %v", name, err)
		}
		if name != "disabled" {
			d := ee.Details.(map[string]any)
			if d["settings_path"] != "/settings#token-git" || d["host"] != "github.com" {
				t.Fatalf("%s: %v", name, d)
			}
		}
		if n := e.c.count("lock") + e.c.count("upsert") + e.c.count("save") + e.c.count("tree") + e.c.count("audit"); n != 0 {
			t.Fatalf("%s: tulis terjadi %v", name, e.c.list())
		}
	}
}

func TestNoWritesOnEarlyErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		mod  func(*env, *RunRequest)
		code errs.Code
	}{
		"access":    {func(e *env, _ *RunRequest) { e.acc.err = base.ErrProjectNotFound }, errs.CodeNotFound},
		"kind":      {func(_ *env, r *RunRequest) { r.Kind = "x" }, errs.CodeInvalid},
		"http":      {func(_ *env, r *RunRequest) { r.URL = "http://github.com/a/b" }, base.CodeInvalidRepoURL},
		"no branch": {func(_ *env, r *RunRequest) { r.URL = "https://github.com/a/b" }, base.CodeInvalidRepoURL},
		"host":      {func(_ *env, r *RunRequest) { r.URL = "https://git.evil.id/a/b/-/tree/main" }, errs.CodeHostNotAllowed},
		"prefix":    {func(_ *env, r *RunRequest) { r.TargetPrefix = "a/../b" }, errs.CodeInvalid},
	} {
		e := newEnv(t)
		r := kreq()
		tc.mod(e, &r)
		_, err := e.uc.Execute(ctxWithID(), "p1", r, base.RequestMeta{})
		var ee *errs.Error
		if !errors.As(err, &ee) || ee.Code != tc.code {
			t.Fatalf("%s: %v", name, err)
		}
		if n := e.c.count("token") + e.c.count("lock") + e.c.count("upsert") + e.c.count("save"); n != 0 {
			t.Fatalf("%s: %v", name, e.c.list())
		}
	}
}

func TestLockBusy(t *testing.T) {
	e := newEnv(t)
	e.repo.lockOK = false
	_, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != base.CodeSyncInProgress {
		t.Fatal(err)
	}
	if e.c.count("upsert")+e.c.count("tree")+e.c.count("unlock") != 0 {
		t.Fatal(e.c.list())
	}
}

func TestUnlockFailureIgnored(t *testing.T) {
	e := newEnv(t)
	e.repo.unlockEr = errors.New("boom")
	e.addFile("docs/a.md", "a")
	if _, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
}

func TestSpecsPartialAndPrefixIgnored(t *testing.T) {
	e := newEnv(t)
	e.addFile("api/billing.yaml", "x")
	e.addFile("api/pay.json", "y")
	e.addFile("api/readme.txt", "z")
	e.specsResp = func(r upsertspecs.Request) (upsertspecs.Response, error) {
		return upsertspecs.Response{Items: []upsertspecs.ItemResult{
			{Index: 0, SourcePath: "api/billing.yaml", Name: "billing", Status: "created", OpenAPIVersion: "3.0.3", Title: "Billing", EndpointCount: 14},
			{Index: 1, SourcePath: "api/pay.json", Status: "rejected", ReasonCode: "INVALID_SPEC", Reason: "bad",
				Issues: []upsertspecs.Issue{{Code: "EXTERNAL_REF", Message: "m", Pointer: "#/paths/~1c/get", Line: 3}}},
		}}, nil
	}
	req := RunRequest{Kind: "api_spec", URL: "https://github.com/a/b/tree/main/api", TargetPrefix: "../bad"}
	res, err := e.uc.Execute(ctxWithID(), "p1", req, base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Report.Status != "partial" || res.Report.Summary.Rejected != 1 || res.Report.Summary.Skipped != 1 || res.Report.Summary.Created != 1 {
		t.Fatalf("%+v", res.Report)
	}
	if e.upS[0].Items[0].SourcePath != "api/billing.yaml" || e.repo.upserted[0].TargetPrefix != "" {
		t.Fatalf("%+v", e.upS[0])
	}
	for _, it := range res.Report.Items {
		if it.RepoPath == "api/readme.txt" && it.ReasonCode != "NOT_SPEC_FILE" {
			t.Fatalf("%+v", it)
		}
		if it.RepoPath == "api/billing.yaml" && (it.EndpointCount != 14 || it.Target != "billing") {
			t.Fatalf("%+v", it)
		}
		if it.RepoPath == "api/pay.json" && (len(it.Issues) != 1 || it.Issues[0].Pointer != "#/paths/~1c/get") {
			t.Fatalf("%+v", it)
		}
	}
}

func TestExplicitBranchWins(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	r := kreq()
	r.Branch = "feature/x"
	if _, err := e.uc.Execute(ctxWithID(), "p1", r, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if e.repo.upserted[0].Branch != "feature/x" {
		t.Fatalf("%+v", e.repo.upserted[0])
	}
}

func TestBusResultsUnchangedAndTooLarge(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.addFile("docs/b.md", "b")
	e.filesResp = func(r upsertfiles.Request) (upsertfiles.Response, error) {
		return upsertfiles.Response{Items: []upsertfiles.ItemResult{
			{Index: 0, Path: "guides/a.md", Status: "unchanged"},
			{Index: 1, Path: "b.md", Status: "rejected", Reason: "too_large"},
		}}, nil
	}
	res, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	if err != nil || res.Report.Summary.Unchanged != 1 || res.Report.Summary.Rejected != 1 || res.Report.Status != "partial" {
		t.Fatalf("%v %+v", err, res.Report)
	}
}

func TestNoCandidates(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/img.png", "a")
	res, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	if err != nil || res.Report.Status != "success" || res.Report.Summary.Skipped != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	if e.c.count("upsert-") != 0 || e.repo.saved[0].Status != "success" {
		t.Fatal(e.c.list())
	}
}

func TestReportCappedForStorageOnly(t *testing.T) {
	e := newEnv(t)
	e.uc.cfg.MaxUploadFiles = 1000
	e.uc.cfg.MaxUploadBytes = 1 << 30
	for i := range 510 {
		e.addFile(fmt.Sprintf("docs/f%04d.png", i), "x")
	}
	res, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Report.Items) != 510 || res.Report.Summary.Skipped != 510 {
		t.Fatalf("%d", len(res.Report.Items))
	}
	s := e.repo.saved[0].Result
	if len(s.Items) != 500 || !s.Truncated || s.Summary.Skipped != 510 {
		t.Fatalf("%d %v", len(s.Items), s.Truncated)
	}
}

func assertFailed(t *testing.T, e *env, err error, status errs.Code) {
	t.Helper()
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != status {
		t.Fatalf("err=%v want %s", err, status)
	}
	if e.c.count("upsert-") != 0 {
		t.Fatal("bus upsert terpanggil")
	}
	if len(e.repo.saved) != 1 || e.repo.saved[0].Status != "failed" || e.repo.saved[0].Result.ErrorCode != string(status) {
		t.Fatalf("%+v", e.repo.saved)
	}
	if len(e.aud.events) != 1 || e.aud.events[0].ErrorCode != string(status) {
		t.Fatalf("%+v", e.aud.events)
	}
	if e.c.count("unlock") != 1 {
		t.Fatal("unlock")
	}
}

func TestLimits(t *testing.T) {
	e := newEnv(t)
	e.uc.cfg.MaxUploadFiles = 2
	for i := range 3 {
		e.addFile(fmt.Sprintf("docs/f%d.md", i), "x")
	}
	_, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	assertFailed(t, e, err, base.CodeSyncLimitExceeded)
	if e.git.blobCalled.Load() != 0 {
		t.Fatal("fetch terpanggil")
	}

	e = newEnv(t)
	e.uc.cfg.MaxUploadBytes = 5
	e.addFile("docs/a.md", "123456")
	_, err = e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	assertFailed(t, e, err, base.CodeSyncLimitExceeded)
}

func TestCumulativeLimitCancelsWithoutLeak(t *testing.T) {
	e := newEnv(t)
	e.uc.cfg.MaxUploadBytes = 25
	e.uc.cfg.MaxFileBytes = 100
	for i := range 10 {
		sha := fmt.Sprintf("s%d", i)
		e.git.tree = append(e.git.tree, gitprovider.TreeEntry{Path: fmt.Sprintf("docs/f%d.md", i), Size: -1, BlobSHA: sha})
		e.git.blobs[sha] = bytes.Repeat([]byte("x"), 10)
	}
	e.git.delay = 5 * time.Millisecond
	_, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	assertFailed(t, e, err, base.CodeSyncLimitExceeded)
	if e.git.cur.Load() != 0 {
		t.Fatal("goroutine fetch tersisa")
	}
}

func TestOversizedFileNotFetched(t *testing.T) {
	e := newEnv(t)
	e.uc.cfg.MaxFileBytes = 3
	e.addFile("docs/big.md", "123456")
	e.addFile("docs/ok.md", "1")
	e.git.tree = append(e.git.tree, gitprovider.TreeEntry{Path: "docs/unknown.md", Size: -1, BlobSHA: "u"})
	e.git.blobs["u"] = []byte("zzz")
	e.git.tooLarge["u"] = true
	res, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if e.git.blobCalled.Load() != 2 {
		t.Fatalf("fetch=%d", e.git.blobCalled.Load())
	}
	big := 0
	for _, it := range e.upF[0].Items {
		if it.TooLarge {
			big++
			if it.Content != nil {
				t.Fatal("content terkirim")
			}
		}
	}
	if big != 2 || res.Report.Summary.Rejected != 2 {
		t.Fatalf("%d %+v", big, res.Report.Summary)
	}
}

func TestFolderNotFound(t *testing.T) {
	e := newEnv(t)
	e.addFile("other/a.md", "a")
	_, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	assertFailed(t, e, err, base.CodeGitNotFound)
	var ee *errs.Error
	errors.As(err, &ee)
	if ee.Details.(map[string]any)["what"] != "folder" {
		t.Fatal(ee.Details)
	}
}

func TestProviderErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		code errs.Code
	}{
		"401":      {gitprovider.ErrUnauthorized, base.CodeGitTokenRejected},
		"403":      {gitprovider.ErrForbidden, base.CodeGitAccessDenied},
		"404":      {gitprovider.ErrNotFound, base.CodeGitNotFound},
		"rl":       {&gitprovider.RateLimitedError{RetryAfter: 30 * time.Second}, errs.CodeRateLimited},
		"rl plain": {gitprovider.ErrRateLimited, errs.CodeRateLimited},
		"tree":     {gitprovider.ErrTreeTooLarge, base.CodeGitTreeTooLarge},
		"host":     {gitprovider.ErrHostNotAllowed, errs.CodeHostNotAllowed},
		"upstream": {fmt.Errorf("%w: x", gitprovider.ErrUpstream), errs.CodeGitUpstream},
		"deadline": {context.DeadlineExceeded, errs.CodeTimeout},
		"unknown":  {errors.New("pg secret"), errs.CodeInternal},
	}
	for name, tc := range cases {
		for _, where := range []string{"tree", "blob"} {
			e := newEnv(t)
			e.addFile("docs/a.md", "a")
			if where == "tree" {
				e.git.treeErr = tc.err
			} else {
				e.git.blobErr = tc.err
			}
			_, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
			assertFailed(t, e, err, tc.code)
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("%s: bocor %v", name, err)
			}
			if name == "rl" {
				var ee *errs.Error
				errors.As(err, &ee)
				if ee.Details.(map[string]any)["retry_after_seconds"] != 30 {
					t.Fatal(ee.Details)
				}
			}
			if name == "rl plain" {
				var ee *errs.Error
				errors.As(err, &ee)
				if ee.Details != nil {
					t.Fatal(ee.Details)
				}
			}
		}
	}
}

func TestCanceledRecordedAsFailed(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.git.treeErr = context.Canceled
	_, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	if !errors.Is(err, context.Canceled) || e.repo.saved[0].Result.ErrorCode != "CANCELED" || e.repo.saved[0].Status != "failed" {
		t.Fatalf("%v %+v", err, e.repo.saved)
	}
}

func TestDeadlineFinalStepsUseLiveContext(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	ctx, cancel := context.WithCancel(ctxWithID())
	e.git.treeErr = context.DeadlineExceeded
	cancel()
	_, err := e.uc.Execute(ctx, "p1", kreq(), base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeTimeout {
		t.Fatal(err)
	}
	for _, ce := range e.repo.ctxErrs {
		if ce != nil {
			t.Fatalf("ctx mati di langkah akhir: %v", ce)
		}
	}
	if len(e.repo.saved) != 1 || len(e.aud.events) != 1 || e.c.count("unlock") != 1 {
		t.Fatal(e.c.list())
	}
}

func TestFetchConcurrencyBounded(t *testing.T) {
	e := newEnv(t)
	for i := range 10 {
		e.addFile(fmt.Sprintf("docs/f%d.md", i), "x")
	}
	e.git.delay = 20 * time.Millisecond
	if _, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if p := e.git.peak.Load(); p != fetchConcurrency {
		t.Fatalf("peak=%d", p)
	}
}

func TestBusUpsertErrors(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.filesResp = func(upsertfiles.Request) (upsertfiles.Response, error) {
		return upsertfiles.Response{}, errs.New(errs.CodeInvalid, "prefix salah")
	}
	_, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeInvalid || e.repo.saved[0].Status != "failed" {
		t.Fatal(err)
	}

	e = newEnv(t)
	e.addFile("docs/a.md", "a")
	e.filesResp = func(upsertfiles.Request) (upsertfiles.Response, error) {
		return upsertfiles.Response{}, errs.New(errs.CodeNotFound, "pg secret detail")
	}
	_, err = e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	if !errors.As(err, &ee) || ee.Code != errs.CodeInternal || e.repo.saved[0].Result.ErrorCode != "INTERNAL" {
		t.Fatal(err)
	}
}

func TestSaveResultFailureStill200(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.repo.saveErr = errors.New("db down")
	res, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	if err != nil || res.Report.Summary.Created != 1 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestAuditFailOpenAndNoCanary(t *testing.T) {
	scenarios := []func(*env){
		func(e *env) {},
		func(e *env) { e.git.treeErr = gitprovider.ErrUnauthorized },
		func(e *env) { e.git.treeErr = gitprovider.ErrNotFound },
		func(e *env) { e.git.blobErr = &gitprovider.RateLimitedError{RetryAfter: time.Second} },
		func(e *env) { e.git.treeErr = gitprovider.ErrUpstream },
		func(e *env) {
			e.filesResp = func(upsertfiles.Request) (upsertfiles.Response, error) {
				return upsertfiles.Response{}, errors.New("bus 500")
			}
		},
	}
	for i, sc := range scenarios {
		e := newEnv(t)
		e.addFile("docs/a.md", "a")
		sc(e)
		res, err := e.uc.Execute(ctxWithID(), "p1", kreq(), base.RequestMeta{})
		var blob []byte
		if err != nil {
			blob = fmt.Appendf(nil, "%v|%+v", err, err)
		} else {
			blob, _ = json.Marshal(res)
		}
		for _, s := range e.repo.saved {
			j, _ := json.Marshal(s)
			blob = append(blob, j...)
		}
		j, _ := json.Marshal(e.aud.events)
		blob = append(blob, j...)
		blob = append(blob, e.log.Bytes()...)
		if bytes.Contains(blob, []byte("CANARY")) {
			t.Fatalf("scenario %d: canary bocor", i)
		}
	}
}

func TestMapErrorTable(t *testing.T) {
	out, code, _ := mapError(context.Canceled)
	if out != context.Canceled || code != "CANCELED" {
		t.Fatal(out, code)
	}
	pre := errs.New(base.CodeSyncLimitExceeded, "x")
	if out, _, _ := mapError(fmt.Errorf("w: %w", pre)); out != pre {
		t.Fatal("sentinel tidak apa adanya")
	}
}
