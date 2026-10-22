package getconfig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/secret"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
)

type fakeAccess struct {
	actor   base.Actor
	err     error
	project string
}

func (f *fakeAccess) Require(_ context.Context, p string) (base.Actor, error) {
	f.project = p
	if f.err != nil {
		return base.Actor{}, f.err
	}
	if f.actor == (base.Actor{}) {
		return base.Actor{ID: "u1", Role: "developer", Label: "a@tim.id"}, nil
	}
	return f.actor, nil
}

type fakeRepo struct {
	cfg   *base.Config
	err   error
	calls int
	proj  string
	kind  base.Kind

	inProgress    bool
	inProgressErr error
	ipCalls       int
	ipProj        string
	ipKind        base.Kind
}

func (f *fakeRepo) InProgress(_ context.Context, p string, k base.Kind) (bool, error) {
	f.ipCalls++
	f.ipProj, f.ipKind = p, k
	return f.inProgress, f.inProgressErr
}

func (f *fakeRepo) Get(_ context.Context, p string, k base.Kind) (*base.Config, error) {
	f.calls++
	f.proj, f.kind = p, k
	return f.cfg, f.err
}

type fakeCred struct {
	st                     CredentialStatus
	err                    error
	calls                  int
	userID, provider, host string
}

func (f *fakeCred) Status(_ context.Context, u, p, h string) (CredentialStatus, error) {
	f.calls++
	f.userID, f.provider, f.host = u, p, h
	return f.st, f.err
}

func sampleCfg() *base.Config {
	st, by, lbl := "partial", "u-other", "Rina"
	at := time.Date(2026, 10, 22, 14, 5, 0, 0, time.UTC)
	return &base.Config{Kind: base.KindKnowledge, Provider: "github", Host: "github.com", RepoURL: "https://github.com/a/b",
		RepoPath: "a/b", Branch: "main", Folder: "docs", LastStatus: &st, LastSyncedAt: &at, LastSyncedBy: &by, LastSyncedByLabel: &lbl}
}

func newUC(a *fakeAccess, r *fakeRepo, c *fakeCred, enabled bool) (*Usecase, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return NewUsecase(a, r, c, enabled, slog.New(slog.NewTextHandler(buf, nil))), buf
}

func in(kind string) params { return params{ProjectID: "p1", RawKind: kind} }

func TestExecute_ConfigAndCredential(t *testing.T) {
	r, c := &fakeRepo{cfg: sampleCfg()}, &fakeCred{st: CredentialStatus{Found: true}}
	uc, _ := newUC(&fakeAccess{}, r, c, true)
	res, err := uc.Execute(context.Background(), in("knowledge"))
	if err != nil || res.Config == nil || !res.HasCredential || res.CredentialUnreadable || !res.FeatureEnabled || res.Kind != "knowledge" {
		t.Fatalf("%+v %v", res, err)
	}
	if res.Config.LastSyncedBy == nil || res.Config.LastSyncedBy.ID != "u-other" || res.Config.LastSyncedBy.Label != "Rina" {
		t.Fatalf("%+v", res.Config)
	}
	// UserID dari identity request, bukan last_synced_by
	if c.userID != "u1" || c.provider != "github" || c.host != "github.com" {
		t.Fatalf("%+v", c)
	}
}

func TestExecute_NoConfig(t *testing.T) {
	c := &fakeCred{}
	uc, _ := newUC(&fakeAccess{}, &fakeRepo{}, c, true)
	res, err := uc.Execute(context.Background(), in("api_spec"))
	if err != nil || res.Config != nil || res.HasCredential || c.calls != 0 || res.Kind != "api_spec" {
		t.Fatalf("%+v %v calls=%d", res, err, c.calls)
	}
}

func TestExecute_CredentialStates(t *testing.T) {
	cases := map[string]struct {
		st                   CredentialStatus
		err                  error
		has, unread, feature bool
		wantWarn             bool
	}{
		"not found":  {st: CredentialStatus{}, feature: true},
		"unreadable": {st: CredentialStatus{Found: true, Unreadable: true}, unread: true, feature: true},
		"disabled":   {st: CredentialStatus{Found: true, Disabled: true}, feature: false},
		"boom":       {err: errors.New("boom"), feature: true, wantWarn: true},
		"not reg":    {err: bus.ErrNotRegistered, feature: true, wantWarn: true},
	}
	for name, tc := range cases {
		uc, buf := newUC(&fakeAccess{}, &fakeRepo{cfg: sampleCfg()}, &fakeCred{st: tc.st, err: tc.err}, true)
		res, err := uc.Execute(context.Background(), in("knowledge"))
		if err != nil || res.HasCredential != tc.has || res.CredentialUnreadable != tc.unread || res.FeatureEnabled != tc.feature || res.Config == nil {
			t.Errorf("%s: %+v %v", name, res, err)
		}
		if tc.wantWarn != strings.Contains(buf.String(), "level=WARN") {
			t.Errorf("%s: log=%s", name, buf.String())
		}
	}
}

func TestExecute_FeatureDisabledByConfig(t *testing.T) {
	c := &fakeCred{st: CredentialStatus{Found: true}}
	uc, _ := newUC(&fakeAccess{}, &fakeRepo{cfg: sampleCfg()}, c, false)
	res, err := uc.Execute(context.Background(), in("knowledge"))
	if err != nil || c.calls != 0 || res.FeatureEnabled || res.Config == nil || res.HasCredential {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestExecute_AccessErrors(t *testing.T) {
	infra := errors.New("db down")
	for name, tc := range map[string]struct {
		err  error
		kind string
	}{
		"non-member":         {base.ErrProjectNotFound, "knowledge"},
		"unauth":             {base.ErrUnauthenticated, "knowledge"},
		"infra":              {infra, "knowledge"},
		"access before kind": {base.ErrProjectNotFound, "foo"},
	} {
		r, c := &fakeRepo{cfg: sampleCfg()}, &fakeCred{}
		uc, _ := newUC(&fakeAccess{err: tc.err}, r, c, true)
		_, err := uc.Execute(context.Background(), in(tc.kind))
		if !errors.Is(err, tc.err) || r.calls != 0 || c.calls != 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestExecute_InvalidKind(t *testing.T) {
	for _, k := range []string{"", "foo", "Knowledge "} {
		r := &fakeRepo{}
		uc, _ := newUC(&fakeAccess{}, r, &fakeCred{}, true)
		_, err := uc.Execute(context.Background(), in(k))
		if err != base.ErrInvalidKind || r.calls != 0 {
			t.Errorf("%q: %v", k, err)
		}
	}
}

func TestExecute_AdminNonMember(t *testing.T) {
	uc, _ := newUC(&fakeAccess{actor: base.Actor{ID: "sa", Role: "super_admin"}}, &fakeRepo{cfg: sampleCfg()}, &fakeCred{st: CredentialStatus{Found: true}}, true)
	res, err := uc.Execute(context.Background(), in("knowledge"))
	if err != nil || res.Config == nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestExecute_RepoError(t *testing.T) {
	uc, _ := newUC(&fakeAccess{}, &fakeRepo{err: errors.New("pg")}, &fakeCred{}, true)
	_, err := uc.Execute(context.Background(), in("knowledge"))
	var e *errs.Error
	if err == nil || errors.As(err, &e) || !strings.HasPrefix(err.Error(), "getconfig:") {
		t.Fatalf("%v", err)
	}
}

func TestExecute_CapsReport(t *testing.T) {
	items := make([]base.ReportItem, 600)
	for i := range items {
		items[i] = base.ReportItem{RepoPath: fmt.Sprintf("f%03d.md", i), Status: base.ItemCreated}
	}
	cfg := sampleCfg()
	rep := base.NewReport(items)
	cfg.LastResult = &rep
	uc, _ := newUC(&fakeAccess{}, &fakeRepo{cfg: cfg}, &fakeCred{}, true)
	res, err := uc.Execute(context.Background(), in("knowledge"))
	lr := res.Config.LastResult
	if err != nil || len(lr.Items) != 500 || !lr.Truncated || lr.Summary.Created != 600 {
		t.Fatalf("%v items=%d", err, len(lr.Items))
	}
}

func TestExecute_NeverSynced(t *testing.T) {
	cfg := sampleCfg()
	cfg.LastStatus, cfg.LastSyncedAt, cfg.LastSyncedBy, cfg.LastSyncedByLabel = nil, nil, nil, nil
	uc, _ := newUC(&fakeAccess{}, &fakeRepo{cfg: cfg}, &fakeCred{}, true)
	res, _ := uc.Execute(context.Background(), in("knowledge"))
	d := res.Config
	if d.LastResult != nil || d.LastStatus != nil || d.LastSyncedBy != nil || d.LastSyncedAt != nil {
		t.Fatalf("%+v", d)
	}
}

func busWith(h func(context.Context, any) (any, error)) *bus.Registry {
	b := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.Register(constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, h)
	return b
}

func TestBusCredentials_NoTokenLeak(t *testing.T) {
	const canary = "ghp_CANARYSECRET123"
	var got any
	b := busWith(func(_ context.Context, req any) (any, error) {
		got = req
		return gettoken.Response{Found: true, Token: secret.New(canary)}, nil
	})
	buf := &bytes.Buffer{}
	uc := NewUsecase(&fakeAccess{}, &fakeRepo{cfg: sampleCfg()}, busCredentials{b: b}, true, slog.New(slog.NewTextHandler(buf, nil)))
	res, err := uc.Execute(context.Background(), in("knowledge"))
	if err != nil || !res.HasCredential {
		t.Fatalf("%+v %v", res, err)
	}
	if _, ok := got.(gettoken.Request); !ok {
		t.Fatalf("request harus nilai: %T", got)
	}
	if strings.Contains(fmt.Sprintf("%+v", res), canary) || strings.Contains(buf.String(), canary) {
		t.Fatal("token bocor")
	}
	rt := reflect.TypeOf(CredentialStatus{})
	var names []string
	for i := 0; i < rt.NumField(); i++ {
		names = append(names, rt.Field(i).Name)
	}
	if strings.Join(names, ",") != "Found,Unreadable,Disabled" {
		t.Fatal(names)
	}
}

func TestBusCredentials_WrongTypeFailSoft(t *testing.T) {
	b := busWith(func(context.Context, any) (any, error) { return "salah", nil })
	uc, buf := newUC(&fakeAccess{}, &fakeRepo{cfg: sampleCfg()}, nil, true)
	uc.cred = busCredentials{b: b}
	res, err := uc.Execute(context.Background(), in("knowledge"))
	if err != nil || res.HasCredential || !strings.Contains(buf.String(), "WARN") {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestExecute_InProgress(t *testing.T) {
	r := &fakeRepo{cfg: sampleCfg(), inProgress: true}
	uc, _ := newUC(&fakeAccess{}, r, &fakeCred{st: CredentialStatus{Found: true}}, true)
	res, err := uc.Execute(context.Background(), in("api_spec"))
	if err != nil || !res.InProgress || res.Config == nil || !res.HasCredential {
		t.Fatalf("%+v %v", res, err)
	}
	if r.ipProj != "p1" || r.ipKind != base.KindAPISpec {
		t.Fatalf("%q %q", r.ipProj, r.ipKind)
	}
	uc, _ = newUC(&fakeAccess{}, &fakeRepo{cfg: sampleCfg()}, &fakeCred{}, true)
	if res, _ := uc.Execute(context.Background(), in("knowledge")); res.InProgress {
		t.Fatal("harus false")
	}
}

func TestExecute_InProgressNoConfig(t *testing.T) {
	c := &fakeCred{}
	uc, _ := newUC(&fakeAccess{}, &fakeRepo{inProgress: true}, c, true)
	res, err := uc.Execute(context.Background(), in("knowledge"))
	if err != nil || res.Config != nil || !res.InProgress || c.calls != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestExecute_InProgressFailSoft(t *testing.T) {
	c := &fakeCred{st: CredentialStatus{Found: true}}
	uc, buf := newUC(&fakeAccess{}, &fakeRepo{cfg: sampleCfg(), inProgress: true, inProgressErr: errors.New("boom")}, c, true)
	res, err := uc.Execute(context.Background(), in("knowledge"))
	if err != nil || res.InProgress || res.Config == nil || !res.HasCredential {
		t.Fatalf("%+v %v", res, err)
	}
	if l := buf.String(); !strings.Contains(l, "level=WARN") || !strings.Contains(l, "project_id=p1") || !strings.Contains(l, "kind=knowledge") {
		t.Fatalf("log=%s", l)
	}
}

func TestExecute_InProgressNotCalledEarly(t *testing.T) {
	for name, tc := range map[string]struct {
		a    *fakeAccess
		r    *fakeRepo
		kind string
	}{
		"access":   {&fakeAccess{err: base.ErrProjectNotFound}, &fakeRepo{}, "knowledge"},
		"bad kind": {&fakeAccess{}, &fakeRepo{}, "foo"},
		"get err":  {&fakeAccess{}, &fakeRepo{err: errors.New("x")}, "knowledge"},
	} {
		uc, _ := newUC(tc.a, tc.r, &fakeCred{}, true)
		if _, err := uc.Execute(context.Background(), in(tc.kind)); err == nil || tc.r.ipCalls != 0 {
			t.Errorf("%s: err=%v ip=%d", name, err, tc.r.ipCalls)
		}
	}
}
