package deletecredential

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

type delCall struct {
	user string
	p    base.Provider
	host string
}

type fakeRepo struct {
	calls   []delCall
	deleted bool
	err     error
	log     *[]string
}

func (f *fakeRepo) Delete(_ context.Context, u string, p base.Provider, h string) (bool, error) {
	f.calls = append(f.calls, delCall{u, p, h})
	if f.log != nil {
		*f.log = append(*f.log, "delete")
	}
	return f.deleted, f.err
}

type fakeAudit struct {
	events []audit.Event
	log    *[]string
}

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) {
	f.events = append(f.events, ev)
	if f.log != nil {
		*f.log = append(*f.log, "audit")
	}
}

func ctxFor(id string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Name: "Ani"})
}

func run(t *testing.T, ctx context.Context, repo *fakeRepo, in Input) (error, *fakeAudit) {
	t.Helper()
	a := &fakeAudit{}
	return NewUsecase(repo, a).Execute(ctx, in), a
}

func code(err error) errs.Code {
	var e *errs.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestExecute_Success(t *testing.T) {
	cases := []struct {
		name, provider, host string
		wantP                base.Provider
		wantHost             string
	}{
		{"github tanpa host", "github", "", base.ProviderGitHub, "github.com"},
		{"github host eksplisit", "github", "github.com", base.ProviderGitHub, "github.com"},
		{"github api host", "github", "api.github.com", base.ProviderGitHub, "github.com"},
		{"gitlab kosong", "gitlab", "", base.ProviderGitLab, "gitlab.com"},
		{"gitlab self-hosted", "gitlab", "HTTPS://Git.Noobee.ID/", base.ProviderGitLab, "git.noobee.id"},
		{"provider huruf besar", "GitHub", "", base.ProviderGitHub, "github.com"},
		{"host di luar allowlist tetap terhapus", "gitlab", "git.dikeluarkan.id", base.ProviderGitLab, "git.dikeluarkan.id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{deleted: true}
			err, a := run(t, ctxFor("u1"), repo, Input{Provider: c.provider, Host: c.host})
			if err != nil {
				t.Fatal(err)
			}
			if len(repo.calls) != 1 || repo.calls[0] != (delCall{"u1", c.wantP, c.wantHost}) {
				t.Fatalf("calls %+v", repo.calls)
			}
			if len(a.events) != 1 {
				t.Fatalf("audit %d", len(a.events))
			}
			ev := a.events[0]
			if ev.Action != record.ActionGitCredentialDeleted || ev.Provider != c.wantP || ev.Host != c.wantHost || ev.Actor.ID != "u1" {
				t.Fatalf("event %+v", ev)
			}
		})
	}
}

func TestExecute_Rejections(t *testing.T) {
	cases := []struct {
		name, provider, host string
		want                 errs.Code
	}{
		{"github host asing", "github", "evil.com", errs.CodeHostNotAllowed},
		{"provider tidak dikenal", "bitbucket", "", errs.CodeInvalid},
		{"provider kosong", "", "", errs.CodeInvalid},
		{"gitlab userinfo", "gitlab", "user@host", errs.CodeInvalid},
		{"gitlab path", "gitlab", "host/path", errs.CodeInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{deleted: true}
			err, a := run(t, ctxFor("u1"), repo, Input{Provider: c.provider, Host: c.host})
			if code(err) != c.want {
				t.Fatalf("err %v", err)
			}
			if len(repo.calls) != 0 || len(a.events) != 0 {
				t.Fatalf("repo %d audit %d", len(repo.calls), len(a.events))
			}
		})
	}
}

func TestExecute_NotFound(t *testing.T) {
	repo := &fakeRepo{}
	err, a := run(t, ctxFor("u1"), repo, Input{Provider: "github"})
	if !errors.Is(err, base.ErrCredentialNotFound) {
		t.Fatalf("err %v", err)
	}
	if len(a.events) != 0 {
		t.Fatal("audit tidak boleh tercatat")
	}
}

func TestExecute_NoLogin(t *testing.T) {
	repo := &fakeRepo{deleted: true}
	err, a := run(t, context.Background(), repo, Input{Provider: "github"})
	if code(err) != errs.CodeUnauthorized || len(repo.calls) != 0 || len(a.events) != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestExecute_RepoError(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db down")}
	err, a := run(t, ctxFor("u1"), repo, Input{Provider: "github"})
	if err == nil || !strings.HasPrefix(err.Error(), "deletecredential: delete:") {
		t.Fatalf("err %v", err)
	}
	var e *errs.Error
	if errors.As(err, &e) {
		t.Fatal("harus bukan *errs.Error")
	}
	if len(a.events) != 0 {
		t.Fatal("audit tidak boleh tercatat")
	}
}

func TestExecute_ActorFromIdentity(t *testing.T) {
	repo := &fakeRepo{deleted: true}
	if err, _ := run(t, ctxFor("u9"), repo, Input{Provider: "github"}); err != nil {
		t.Fatal(err)
	}
	if repo.calls[0].user != "u9" {
		t.Fatalf("user %s", repo.calls[0].user)
	}
}

func TestExecute_AuditAfterDelete(t *testing.T) {
	var order []string
	repo := &fakeRepo{deleted: true, log: &order}
	a := &fakeAudit{log: &order}
	if err := NewUsecase(repo, a).Execute(ctxFor("u1"), Input{Provider: "github"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "delete,audit" {
		t.Fatalf("order %v", order)
	}
}

func TestExecute_AuditEventHasNoSecrets(t *testing.T) {
	repo := &fakeRepo{deleted: true}
	_, a := run(t, ctxFor("u1"), repo, Input{Provider: "gitlab", Host: "git.noobee.id"})
	b, _ := json.Marshal(a.events[0].Args)
	if len(a.events[0].Args) != 0 {
		t.Fatalf("args tambahan: %s", b)
	}
	for _, bad := range []string{"token", "last4", "user_id"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("args memuat %s", bad)
		}
	}
}
