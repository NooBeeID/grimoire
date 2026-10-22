//go:build integration

package bustoken

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/crypto/secretbox"
	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
	"grimoire/migrations"
)

func setup(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func seedUser(t *testing.T, d *db.DB) string {
	t.Helper()
	var id string
	email := uuid.NewString() + "@x.id"
	if err := d.GetContext(context.Background(), &id,
		`INSERT INTO users (email,name,password_hash,role) VALUES ($1,'N','h','developer') RETURNING id`, email); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedCred(t *testing.T, d *db.DB, user string, p base.Provider, host string, blob []byte) {
	t.Helper()
	if _, err := d.Exec(`INSERT INTO git_credentials (user_id,provider,host,token_enc,last4) VALUES ($1,$2,$3,$4,'abcd')`,
		user, string(p), host, blob); err != nil {
		t.Fatal(err)
	}
}

func sealFor(t *testing.T, box *secretbox.Box, user string, p base.Provider, host, plain string) []byte {
	t.Helper()
	b, err := box.Seal([]byte(plain), base.AAD(user, p, host))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRepositoryIsolation_Integration(t *testing.T) {
	d := setup(t)
	repo := NewRepository(d)
	ctx := context.Background()
	a, b, c := seedUser(t, d), seedUser(t, d), seedUser(t, d)
	seedCred(t, d, a, base.ProviderGitHub, "github.com", []byte(strings.Repeat("A", 40)))
	seedCred(t, d, b, base.ProviderGitHub, "github.com", []byte(strings.Repeat("B", 40)))
	seedCred(t, d, a, base.ProviderGitLab, "gitlab.com", []byte(strings.Repeat("C", 40)))
	seedCred(t, d, a, base.ProviderGitLab, "git.noobee.id", []byte(strings.Repeat("D", 40)))

	type tc struct {
		name  string
		user  string
		p     base.Provider
		host  string
		want  string
		found bool
	}
	for _, x := range []tc{
		{"user A", a, base.ProviderGitHub, "github.com", "A", true},
		{"user B", b, base.ProviderGitHub, "github.com", "B", true},
		{"user tanpa token", c, base.ProviderGitHub, "github.com", "", false},
		{"host spesifik", a, base.ProviderGitLab, "git.noobee.id", "D", true},
		{"host lain", a, base.ProviderGitLab, "other.host", "", false},
		{"provider lain", c, base.ProviderGitLab, "gitlab.com", "", false},
		{"user tidak ada", uuid.NewString(), base.ProviderGitHub, "github.com", "", false},
	} {
		blob, found, err := repo.Get(ctx, x.user, x.p, x.host)
		if err != nil || found != x.found || (x.found && string(blob) != strings.Repeat(x.want, 40)) {
			t.Fatalf("%s: %q %v %v", x.name, blob, found, err)
		}
	}
}

func TestBusEndToEnd_Integration(t *testing.T) {
	d := setup(t)
	k1, k2 := newBox(t, 1), newBox(t, 2)
	a, b := seedUser(t, d), seedUser(t, d)
	seedCred(t, d, a, base.ProviderGitHub, "github.com", sealFor(t, k1, a, base.ProviderGitHub, "github.com", canary))
	seedCred(t, d, b, base.ProviderGitHub, "github.com", sealFor(t, k1, b, base.ProviderGitHub, "github.com", "ghp_otherUserToken0001"))

	mk := func(box *secretbox.Box) *bus.Registry {
		r := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
		Init(app.Deps{DB: d, Bus: r, Log: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))}, base.Shared{Box: box})
		return r
	}
	req := func(u string) gettoken.Request {
		return gettoken.Request{UserID: u, Provider: "github", Host: "github.com"}
	}

	res, err := call(mk(k1), req(a))
	if err != nil || !res.Found || res.Token.Reveal() != canary {
		t.Fatalf("%+v %v", res, err)
	}
	if res, _ = call(mk(k1), req(b)); res.Token.Reveal() == canary {
		t.Fatal("token user lain bocor")
	}
	if res, err = call(mk(k2), req(a)); err != nil || !res.Found || !res.Unreadable || !res.Token.IsZero() {
		t.Fatalf("%+v %v", res, err)
	}
	if res, err = call(mk(nil), req(a)); err != nil || !res.Disabled {
		t.Fatalf("%+v %v", res, err)
	}

	var raw string
	if err := d.GetContext(context.Background(), &raw, `SELECT token_enc::text FROM git_credentials WHERE user_id=$1`, a); err != nil || strings.Contains(raw, canary) {
		t.Fatalf("plaintext di DB: %v", err)
	}
}
