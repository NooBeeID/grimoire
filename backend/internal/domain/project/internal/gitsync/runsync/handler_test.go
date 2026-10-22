package runsync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/gitprovider"
	httpx "grimoire/internal/infra/http"
	"grimoire/lib/common/errs"
)

func init() {
	// kode milik module didaftarkan di gitsync.registerCodes; replikasi aman untuk tes paket ini.
	for code, status := range map[errs.Code]int{
		base.CodeCredentialMissing: 422, base.CodeCredentialUnreadable: 422, base.CodeInvalidRepoURL: 400,
		base.CodeGitTokenRejected: 422, base.CodeGitAccessDenied: 422, base.CodeGitNotFound: 404,
		base.CodeSyncLimitExceeded: 422, base.CodeSyncInProgress: 409, base.CodeGitTreeTooLarge: 422,
	} {
		httpx.RegisterCodeStatus(code, status)
	}
}

func serve(e *env, body string, ctx context.Context) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/git-sync/run", newHandler(e.uc, handlerOpts{heartbeat: 20 * time.Millisecond}).ServeHTTP)
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/git-sync/run", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("User-Agent", "agent/1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

const okBody = `{"kind":"knowledge","url":"https://github.com/noobee/grimoire-docs/tree/main/docs","target_prefix":"guides"}`

func TestHandlerOK(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	rec := serve(e, okBody, ctxWithID())
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	b := rec.Body.String()
	for _, w := range []string{`"report"`, `"config"`, `"repo_path":"docs/a.md"`, `"last_result":null`, `"target_prefix":"guides"`} {
		if !strings.Contains(b, w) {
			t.Errorf("missing %s: %s", w, b)
		}
	}
	if e.aud.events[0].Meta.UserAgent != "agent/1" || e.access() != "p1" {
		t.Fatalf("%+v", e.aud.events[0].Meta)
	}
}

func (e *env) access() string { return e.aud.events[0].ProjectID }

func TestHandlerErrors(t *testing.T) {
	type tc struct {
		name   string
		body   string
		ctx    context.Context
		mod    func(*env)
		status int
		code   string
	}
	for _, c := range []tc{
		{"unauth", okBody, context.Background(), func(e *env) { e.acc.err = base.ErrUnauthenticated }, 401, "UNAUTHORIZED"},
		{"non member", okBody, ctxWithID(), func(e *env) { e.acc.err = base.ErrProjectNotFound }, 404, "NOT_FOUND"},
		{"bad json", `{`, ctxWithID(), nil, 400, "INVALID_ARGUMENT"},
		{"unknown field", `{"kind":"knowledge","x":1}`, ctxWithID(), nil, 400, "INVALID_ARGUMENT"},
		{"bad kind", `{"kind":"zzz","url":"https://github.com/a/b"}`, ctxWithID(), nil, 400, "INVALID_ARGUMENT"},
		{"bad url", `{"kind":"knowledge","url":"http://x"}`, ctxWithID(), nil, 400, "INVALID_REPO_URL"},
		{"bad prefix", `{"kind":"knowledge","url":"https://github.com/a/b/tree/main","target_prefix":"a/../b"}`, ctxWithID(), nil, 400, "INVALID_ARGUMENT"},
		{"host", `{"kind":"knowledge","url":"https://git.evil.id/a/b/-/tree/main"}`, ctxWithID(), nil, 422, "HOST_NOT_ALLOWED"},
		{"missing token", okBody, ctxWithID(), func(e *env) { e.tokResp.Found = false }, 422, "CREDENTIAL_MISSING"},
		{"disabled", okBody, ctxWithID(), func(e *env) { e.tokResp.Disabled = true }, 503, "GIT_FEATURE_DISABLED"},
		{"busy", okBody, ctxWithID(), func(e *env) { e.repo.lockOK = false }, 409, "SYNC_IN_PROGRESS"},
		{"401 provider", okBody, ctxWithID(), func(e *env) { e.git.treeErr = gitprovider.ErrUnauthorized }, 422, "GIT_TOKEN_REJECTED"},
		{"repo 404", okBody, ctxWithID(), func(e *env) { e.git.treeErr = gitprovider.ErrNotFound }, 404, "GIT_NOT_FOUND"},
		{"upstream", okBody, ctxWithID(), func(e *env) { e.git.treeErr = gitprovider.ErrUpstream }, 502, "GIT_UPSTREAM_ERROR"},
		{"timeout", okBody, ctxWithID(), func(e *env) { e.git.treeErr = context.DeadlineExceeded }, 504, "TIMEOUT"},
		{"internal", okBody, ctxWithID(), func(e *env) { e.git.treeErr = context.Canceled; e.git.treeErr = errSecret }, 500, "INTERNAL"},
	} {
		e := newEnv(t)
		e.addFile("docs/a.md", "a")
		if c.mod != nil {
			c.mod(e)
		}
		rec := serve(e, c.body, c.ctx)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), `"code":"`+c.code+`"`) || strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%s: envelope %s", c.name, rec.Body)
		}
	}
}

var errSecret = &secretErr{}

type secretErr struct{}

func (*secretErr) Error() string { return "pg secret detail" }

func TestHandlerRetryAfter(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.git.treeErr = &gitprovider.RateLimitedError{RetryAfter: 60 * time.Second}
	rec := serve(e, okBody, ctxWithID())
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	e = newEnv(t)
	e.addFile("docs/a.md", "a")
	e.git.treeErr = gitprovider.ErrRateLimited
	rec = serve(e, okBody, ctxWithID())
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
}
