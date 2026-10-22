package listcredentials

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeRepo struct {
	creds  []base.Credential
	err    error
	calls  int
	userID string
}

func (f *fakeRepo) ListByUser(_ context.Context, id string) ([]base.Credential, error) {
	f.calls++
	f.userID = id
	return f.creds, f.err
}

func authCtxAs(id, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Role: role, Email: id + "@tim.id"})
}

var t0 = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

func TestExecute_Unauthorized(t *testing.T) {
	r := &fakeRepo{}
	_, err := NewUsecase(r, Settings{}).Execute(context.Background())
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized || r.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}

func TestExecute_UsesOwnIdentityEvenForAdmin(t *testing.T) {
	for _, role := range []string{"developer", "super_admin"} {
		r := &fakeRepo{}
		if _, err := NewUsecase(r, Settings{}).Execute(authCtxAs("u1", role)); err != nil {
			t.Fatal(err)
		}
		if r.userID != "u1" || r.calls != 1 {
			t.Fatalf("role=%s userID=%q calls=%d", role, r.userID, r.calls)
		}
	}
}

func TestExecute_MapsDTO(t *testing.T) {
	r := &fakeRepo{creds: []base.Credential{
		{Provider: base.Provider("github"), Host: "github.com", Last4: "7Qx2", CreatedAt: t0, UpdatedAt: t0.Add(time.Hour)},
		{Provider: base.Provider("gitlab"), Host: "git.noobee.id", Last4: "abcd", CreatedAt: t0, UpdatedAt: t0},
	}}
	res, err := NewUsecase(r, Settings{Enabled: true, GitLabHosts: []string{"gitlab.com", "git.noobee.id"}}).Execute(authCtxAs("u1", "developer"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 || res.Items[0].Provider != "github" || res.Items[0].Last4 != "7Qx2" ||
		!res.Items[0].UpdatedAt.Equal(t0.Add(time.Hour)) || res.Items[1].Host != "git.noobee.id" {
		t.Fatalf("items=%+v", res.Items)
	}
	if !res.Enabled || len(res.AllowedGitLabHosts) != 2 || res.AllowedGitLabHosts[1] != "git.noobee.id" {
		t.Fatalf("res=%+v", res)
	}
}

func TestExecute_EmptyIsArray(t *testing.T) {
	for _, creds := range [][]base.Credential{nil, {}} {
		res, err := NewUsecase(&fakeRepo{creds: creds}, Settings{}).Execute(authCtxAs("u1", "developer"))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(res)
		if !strings.Contains(string(b), `"items":[]`) || !strings.Contains(string(b), `"allowed_gitlab_hosts":[]`) {
			t.Fatalf("json=%s", b)
		}
	}
}

func TestExecute_DisabledStillReturnsItems(t *testing.T) {
	r := &fakeRepo{creds: []base.Credential{{Provider: "github", Host: "github.com", Last4: "1234", CreatedAt: t0, UpdatedAt: t0}}}
	res, err := NewUsecase(r, Settings{Enabled: false}).Execute(authCtxAs("u1", "developer"))
	if err != nil || res.Enabled || len(res.Items) != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestNewUsecase_NoAliasing(t *testing.T) {
	src := []string{"gitlab.com"}
	uc := NewUsecase(&fakeRepo{}, Settings{GitLabHosts: src})
	src[0] = "evil.example"
	res, _ := uc.Execute(authCtxAs("u1", "developer"))
	if res.AllowedGitLabHosts[0] != "gitlab.com" {
		t.Fatalf("hosts=%v", res.AllowedGitLabHosts)
	}
}

func TestExecute_RepoErrorWrapped(t *testing.T) {
	boom := errors.New("boom")
	res, err := NewUsecase(&fakeRepo{err: boom}, Settings{Enabled: true}).Execute(authCtxAs("u1", "developer"))
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) || res.Enabled || res.Items != nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestExecute_NoTokenInOutput(t *testing.T) {
	r := &fakeRepo{creds: []base.Credential{{Provider: "github", Host: "github.com", Last4: "1234", TokenEnc: []byte("CANARY-secret"), CreatedAt: t0, UpdatedAt: t0}}}
	res, _ := NewUsecase(r, Settings{}).Execute(authCtxAs("u1", "developer"))
	b, _ := json.Marshal(res)
	low := strings.ToLower(string(b))
	for _, bad := range []string{"canary", "token"} {
		if strings.Contains(low, bad) {
			t.Fatalf("leak %q in %s", bad, b)
		}
	}
}

func TestRepositorySQL_NoTokenEnc(t *testing.T) {
	if strings.Contains(listByUserSQL, "token_enc") {
		t.Fatal("SQL membaca token_enc")
	}
	src, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "SELECT provider, host, last4, created_at, updated_at") {
		t.Fatal("kolom SELECT berubah")
	}
}

func TestRepo_InvalidUUIDNoQuery(t *testing.T) {
	got, err := newRepo(nil).ListByUser(context.Background(), "bukan-uuid")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
