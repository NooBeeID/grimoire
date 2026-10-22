package savecredential

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/base/audit"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/crypto/secretbox"
	"grimoire/internal/infra/gitprovider"
	"grimoire/internal/infra/safehttp"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/common/secret"
	"grimoire/lib/contracts/core/audit/record"
)

const (
	canary = "ghp_CANARY0123456789abcdefCANARY"
	userID = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
)

type vcall struct {
	p     gitprovider.Provider
	host  string
	token string
}
type fakeVerifier struct {
	err   error
	calls []vcall
}

func (f *fakeVerifier) VerifyToken(_ context.Context, p gitprovider.Provider, host string, t secret.String) error {
	f.calls = append(f.calls, vcall{p, host, t.Reveal()})
	return f.err
}

type fakeRepo struct {
	saved base.Saved
	err   error
	got   []base.Credential
}

func (f *fakeRepo) Upsert(_ context.Context, c base.Credential) (base.Saved, error) {
	f.got = append(f.got, c)
	if f.err != nil {
		return base.Saved{}, f.err
	}
	return f.saved, nil
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) { f.events = append(f.events, ev) }

type fakePolicy struct{ allow map[string]bool }

func (f fakePolicy) Allowed(p gitprovider.Provider, host string) bool {
	return f.allow[string(p)+"|"+host]
}

type failSealer struct{ calls int }

func (f *failSealer) Seal(_, _ []byte) ([]byte, error) {
	f.calls++
	return nil, errors.New("seal boom")
}

type countSealer struct {
	inner Sealer
	calls int
}

func (c *countSealer) Seal(p, a []byte) ([]byte, error) { c.calls++; return c.inner.Seal(p, a) }

type env struct {
	uc  *Usecase
	ver *fakeVerifier
	rep *fakeRepo
	aud *fakeAudit
	box *secretbox.Box
	sl  *countSealer
	log *bytes.Buffer
}

func newEnv(t *testing.T, enabled bool) *env {
	t.Helper()
	box, err := secretbox.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	e := &env{
		ver: &fakeVerifier{},
		rep: &fakeRepo{saved: base.Saved{CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0)}},
		aud: &fakeAudit{},
		box: box,
		log: &bytes.Buffer{},
	}
	e.sl = &countSealer{inner: box}
	pol := fakePolicy{allow: map[string]bool{
		"github|github.com": true, "gitlab|gitlab.com": true,
		"gitlab|git.noobee.id": true, "gitlab|git.noobee.id:8443": true,
	}}
	lg := slog.New(slog.NewJSONHandler(e.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.uc = NewUsecase(enabled, pol, e.ver, e.sl, e.rep, e.aud, base.SystemClock, lg)
	return e
}

func ctxUser() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: userID, Email: "a@x.id", Name: "A"})
}

func req(host, token string) SaveRequest { return SaveRequest{Host: host, Token: secret.New(token)} }

func code(t *testing.T, err error) errs.Code {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	return e.Code
}

func (e *env) noSideEffects(t *testing.T) {
	t.Helper()
	if len(e.ver.calls) != 0 || len(e.rep.got) != 0 || len(e.aud.events) != 0 || e.sl.calls != 0 {
		t.Fatalf("ada efek samping: verifier=%d repo=%d audit=%d seal=%d",
			len(e.ver.calls), len(e.rep.got), len(e.aud.events), e.sl.calls)
	}
}

func TestExecute_GitHubValid(t *testing.T) {
	e := newEnv(t, true)
	res, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.ver.calls) != 1 || e.ver.calls[0] != (vcall{gitprovider.GitHub, "github.com", canary}) {
		t.Fatalf("verifier calls: %+v", e.ver.calls)
	}
	c := e.rep.got[0]
	if c.UserID != userID || c.Provider != base.ProviderGitHub || c.Host != "github.com" || c.Last4 != "NARY" {
		t.Fatalf("credential: %+v", c)
	}
	if bytes.Contains(c.TokenEnc, []byte(canary)) {
		t.Fatal("token enc memuat plaintext")
	}
	plain, err := e.box.Open(c.TokenEnc, base.AAD(userID, "github", "github.com"))
	if err != nil || string(plain) != canary {
		t.Fatalf("open: %v %q", err, plain)
	}
	if len(e.aud.events) != 1 || res.Replaced || res.Last4 != "NARY" || res.Host != "github.com" {
		t.Fatalf("hasil: %+v audit=%d", res, len(e.aud.events))
	}
}

func TestExecute_ProviderCaseInsensitive(t *testing.T) {
	e := newEnv(t, true)
	if _, err := e.uc.Execute(ctxUser(), "GitHub", req("", canary), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if e.rep.got[0].Provider != base.ProviderGitHub {
		t.Fatal(e.rep.got[0].Provider)
	}
}

func TestExecute_GitHubHostForced(t *testing.T) {
	for _, h := range []string{"api.github.com", ""} {
		e := newEnv(t, true)
		if _, err := e.uc.Execute(ctxUser(), "github", req(h, canary), base.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if e.rep.got[0].Host != "github.com" {
			t.Fatal(e.rep.got[0].Host)
		}
	}
	e := newEnv(t, true)
	_, err := e.uc.Execute(ctxUser(), "github", req("evil.com", canary), base.RequestMeta{})
	if code(t, err) != errs.CodeHostNotAllowed {
		t.Fatal(err)
	}
	e.noSideEffects(t)
}

func TestExecute_GitLabDefaultHost(t *testing.T) {
	e := newEnv(t, true)
	if _, err := e.uc.Execute(ctxUser(), "gitlab", req("", canary), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if e.rep.got[0].Host != "gitlab.com" {
		t.Fatal(e.rep.got[0].Host)
	}
}

func TestExecute_GitLabSelfHostedAllowed(t *testing.T) {
	e := newEnv(t, true)
	if _, err := e.uc.Execute(ctxUser(), "gitlab", req("HTTPS://Git.Noobee.ID/", canary), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if e.ver.calls[0].host != "git.noobee.id" || e.rep.got[0].Host != "git.noobee.id" {
		t.Fatalf("%+v %+v", e.ver.calls, e.rep.got)
	}
}

func TestExecute_GitLabSelfHostedNotAllowed(t *testing.T) {
	e := newEnv(t, true)
	_, err := e.uc.Execute(ctxUser(), "gitlab", req("git.contoh.com", canary), base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeHostNotAllowed {
		t.Fatal(err)
	}
	if d, _ := ee.Details.(map[string]any); d["host"] != "git.contoh.com" {
		t.Fatalf("details: %+v", ee.Details)
	}
	e.noSideEffects(t)
}

func TestExecute_GitLabHostWithPortAllowed(t *testing.T) {
	e := newEnv(t, true)
	if _, err := e.uc.Execute(ctxUser(), "gitlab", req("git.noobee.id:8443", canary), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	c := e.rep.got[0]
	if _, err := e.box.Open(c.TokenEnc, base.AAD(userID, "gitlab", "git.noobee.id:8443")); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_HostWithUserinfoInvalid(t *testing.T) {
	e := newEnv(t, true)
	_, err := e.uc.Execute(ctxUser(), "gitlab", req("tok:secret@git.noobee.id", canary), base.RequestMeta{})
	if code(t, err) != errs.CodeInvalid {
		t.Fatal(err)
	}
	var ee *errs.Error
	errors.As(err, &ee)
	if strings.Contains(fmt.Sprintf("%v %+v", ee.Message, ee.Details), "secret") {
		t.Fatal("error memantulkan userinfo")
	}
	e.noSideEffects(t)
}

func TestExecute_InvalidProvider(t *testing.T) {
	for _, p := range []string{"bitbucket", ""} {
		e := newEnv(t, true)
		_, err := e.uc.Execute(ctxUser(), p, req("", canary), base.RequestMeta{})
		var ee *errs.Error
		if !errors.As(err, &ee) || ee.Code != errs.CodeInvalid || !strings.Contains(fmt.Sprint(ee.Details), "provider") {
			t.Fatal(err)
		}
		e.noSideEffects(t)
	}
}

func TestExecute_InvalidToken(t *testing.T) {
	for _, tok := range []string{"1234567", strings.Repeat("a", 513), "abcd efgh", "abcdefghéé", ""} {
		e := newEnv(t, true)
		_, err := e.uc.Execute(ctxUser(), "github", req("", tok), base.RequestMeta{})
		var ee *errs.Error
		if !errors.As(err, &ee) || ee.Code != errs.CodeInvalid || !strings.Contains(fmt.Sprint(ee.Details), "token") {
			t.Fatalf("%q: %v", tok, err)
		}
		if tok != "" && strings.Contains(fmt.Sprintf("%v %+v", ee.Message, ee.Details), tok) {
			t.Fatal("error memuat token")
		}
		e.noSideEffects(t)
	}
}

func TestExecute_TokenTrimmed(t *testing.T) {
	e := newEnv(t, true)
	if _, err := e.uc.Execute(ctxUser(), "github", req("", "  "+canary+"\n"), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if e.ver.calls[0].token != canary {
		t.Fatalf("%q", e.ver.calls[0].token)
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	e := newEnv(t, true)
	_, err := e.uc.Execute(context.Background(), "github", req("", canary), base.RequestMeta{})
	if code(t, err) != errs.CodeUnauthorized {
		t.Fatal(err)
	}
	e.noSideEffects(t)
}

func TestExecute_FeatureDisabled(t *testing.T) {
	e := &env{ver: &fakeVerifier{}, rep: &fakeRepo{}, aud: &fakeAudit{}, sl: &countSealer{}}
	e.uc = NewUsecase(false, fakePolicy{}, e.ver, nil, e.rep, e.aud, nil, nil)
	_, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{})
	if code(t, err) != errs.CodeGitFeatureDisabled {
		t.Fatal(err)
	}
	e.noSideEffects(t)
	_, err = e.uc.Execute(context.Background(), "github", req("", canary), base.RequestMeta{})
	if code(t, err) != errs.CodeUnauthorized {
		t.Fatal("identity harus dicek lebih dulu:", err)
	}
}

func TestNewUsecase_EnabledWithoutSealerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("harus panic")
		}
	}()
	NewUsecase(true, fakePolicy{}, &fakeVerifier{}, nil, &fakeRepo{}, &fakeAudit{}, nil, nil)
}

func TestExecute_VerifyFailures(t *testing.T) {
	rl := &gitprovider.RateLimitedError{RetryAfter: 60 * time.Second}
	causes := []error{
		gitprovider.ErrUnauthorized, gitprovider.ErrForbidden, rl, gitprovider.ErrUpstream,
		errors.New("dial tcp: i/o timeout"), context.DeadlineExceeded, safehttp.ErrBlockedAddress,
	}
	for _, c := range causes {
		e := newEnv(t, true)
		e.ver.err = c
		_, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{})
		var vf *VerifyFailure
		if !errors.As(err, &vf) || !errors.Is(err, c) {
			t.Fatalf("%v: %v", c, err)
		}
		if len(e.rep.got) != 0 || len(e.aud.events) != 0 || e.sl.calls != 0 {
			t.Fatal("tidak boleh menyimpan/audit/seal")
		}
	}
	e := newEnv(t, true)
	e.ver.err = rl
	_, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{})
	var got *gitprovider.RateLimitedError
	if !errors.As(err, &got) || got.RetryAfter != 60*time.Second {
		t.Fatal(err)
	}
}

func TestExecute_VerifyFailureErrorString(t *testing.T) {
	e := newEnv(t, true)
	e.ver.err = errors.New("get https://x/?t=" + canary)
	_, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{})
	if err.Error() != "savecredential: verify failed" || strings.Contains(err.Error(), canary) {
		t.Fatal(err)
	}
}

func TestExecute_SealFails(t *testing.T) {
	e := newEnv(t, true)
	e.uc.sealer = &failSealer{}
	_, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{})
	if err == nil || strings.Contains(err.Error(), canary) {
		t.Fatal(err)
	}
	if len(e.rep.got) != 0 || len(e.aud.events) != 0 {
		t.Fatal("tidak boleh simpan/audit")
	}
}

func TestExecute_RepoError(t *testing.T) {
	e := newEnv(t, true)
	e.rep.err = errors.New("db down")
	_, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{})
	if err == nil || err.Error() != "db down" || len(e.aud.events) != 0 {
		t.Fatal(err, len(e.aud.events))
	}
}

func TestExecute_Replaced(t *testing.T) {
	e := newEnv(t, true)
	e.rep.saved.Replaced = true
	res, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{})
	if err != nil || !res.Replaced || e.aud.events[0].Args["replaced"] != true {
		t.Fatal(err, res)
	}
}

func TestExecute_AuditEvent(t *testing.T) {
	e := newEnv(t, true)
	meta := base.RequestMeta{IP: "9.9.9.9", UserAgent: "agent"}
	if _, err := e.uc.Execute(ctxUser(), "github", req("", canary), meta); err != nil {
		t.Fatal(err)
	}
	ev := e.aud.events[0]
	if ev.Action != record.ActionGitCredentialSaved || ev.Actor.ID != userID || ev.Meta != meta {
		t.Fatalf("%+v", ev)
	}
	if len(ev.Args) != 3 || ev.Args["provider"] != "github" || ev.Args["host"] != "github.com" || ev.Args["replaced"] != false {
		t.Fatalf("args: %+v", ev.Args)
	}
}

func TestExecute_AuditFailOpen(t *testing.T) {
	e := newEnv(t, true)
	e.uc.audit = audit.New(bus.NewRegistry(slog.Default()), nil) // bus tanpa handler -> error di-log
	if _, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_OrderOfChecks(t *testing.T) {
	e := newEnv(t, true)
	_, err := e.uc.Execute(ctxUser(), "nope", req("bad host!", "x"), base.RequestMeta{})
	if !strings.Contains(fmt.Sprint(err.(*errs.Error).Details), "provider") {
		t.Fatal(err)
	}
	_, err = e.uc.Execute(ctxUser(), "gitlab", req("bad host!", "x"), base.RequestMeta{})
	if !strings.Contains(fmt.Sprint(err.(*errs.Error).Details), "host") {
		t.Fatal(err)
	}
	_, err = e.uc.Execute(ctxUser(), "gitlab", req("", "x"), base.RequestMeta{})
	if !strings.Contains(fmt.Sprint(err.(*errs.Error).Details), "token") {
		t.Fatal(err)
	}
}

func TestExecute_PATCanary(t *testing.T) {
	parts := []string{canary, canary[:12], canary[len(canary)-12:]}
	check := func(label, s string) {
		for _, p := range parts {
			if strings.Contains(s, p) {
				t.Fatalf("%s memuat canary: %s", label, s)
			}
		}
	}
	for _, cause := range []error{nil, gitprovider.ErrUnauthorized,
		&gitprovider.RateLimitedError{RetryAfter: time.Second}, gitprovider.ErrUpstream,
		errors.New("boom " + "x")} {
		e := newEnv(t, true)
		e.ver.err = cause
		res, err := e.uc.Execute(ctxUser(), "github", req("", canary), base.RequestMeta{})
		check("log", e.log.String())
		check("result", fmt.Sprintf("%+v", res))
		for _, ev := range e.aud.events {
			check("audit", fmt.Sprintf("%+v", ev))
		}
		for _, c := range e.rep.got {
			check("credential", fmt.Sprintf("%+v", c))
		}
		if err != nil {
			check("err", err.Error())
			check("err+v", fmt.Sprintf("%+v", err))
		}
	}
}
