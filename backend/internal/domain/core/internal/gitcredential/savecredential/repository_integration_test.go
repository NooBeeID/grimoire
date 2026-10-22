//go:build integration

package savecredential

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/crypto/secretbox"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
)

func ctxFor(userID string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: userID, Email: "a@x.id", Name: "A"})
}

func setupDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func mkUser(t *testing.T, d *db.DB, email string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id,
		`INSERT INTO users (email,name,password_hash,role) VALUES ($1,'U','x','developer') RETURNING id::text`, email); err != nil {
		t.Fatal(err)
	}
	return id
}

func cred(user string, p base.Provider, host, last4 string) base.Credential {
	return base.Credential{UserID: user, Provider: p, Host: host, TokenEnc: bytes.Repeat([]byte{0xab}, 40), Last4: last4}
}

func rowCount(t *testing.T, d *db.DB, where string, args ...any) int {
	t.Helper()
	var n int
	if err := d.Get(&n, `SELECT count(*) FROM git_credentials WHERE `+where, args...); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepo_UpsertInsertThenReplace(t *testing.T) {
	ctx := context.Background()
	d := setupDB(t)
	r := newRepo(d)
	u := mkUser(t, d, "a@tim.id")

	s1, err := r.Upsert(ctx, cred(u, base.ProviderGitLab, "gitlab.com", "AAAA"))
	if err != nil || s1.Replaced {
		t.Fatalf("insert pertama: replaced=%v err=%v", s1.Replaced, err)
	}
	time.Sleep(5 * time.Millisecond)
	c2 := cred(u, base.ProviderGitLab, "gitlab.com", "BBBB")
	c2.TokenEnc = bytes.Repeat([]byte{0xcd}, 40)
	s2, err := r.Upsert(ctx, c2)
	if err != nil || !s2.Replaced {
		t.Fatalf("penimpaan: replaced=%v err=%v", s2.Replaced, err)
	}
	if !s2.CreatedAt.Equal(s1.CreatedAt) || !s2.UpdatedAt.After(s1.UpdatedAt) {
		t.Fatalf("created_at harus tetap, updated_at maju: %+v %+v", s1, s2)
	}
	var last4 string
	var enc []byte
	if err := d.QueryRow(`SELECT last4, token_enc FROM git_credentials WHERE user_id=$1`, u).Scan(&last4, &enc); err != nil {
		t.Fatal(err)
	}
	if last4 != "BBBB" || !bytes.Equal(enc, c2.TokenEnc) {
		t.Fatalf("baris tidak diganti: %q %x", last4, enc[:2])
	}
	if n := rowCount(t, d, `user_id=$1`, u); n != 1 {
		t.Fatalf("unique (user,provider,host) dilanggar: %d baris", n)
	}
}

func TestRepo_UpsertScopedPerUserProviderHost(t *testing.T) {
	ctx := context.Background()
	d := setupDB(t)
	r := newRepo(d)
	a, b := mkUser(t, d, "a@tim.id"), mkUser(t, d, "b@tim.id")
	for _, c := range []base.Credential{
		cred(a, base.ProviderGitLab, "gitlab.com", "1111"),
		cred(a, base.ProviderGitLab, "git.noobee.id", "2222"),
		cred(a, base.ProviderGitHub, "github.com", "3333"),
		cred(b, base.ProviderGitLab, "gitlab.com", "4444"),
	} {
		s, err := r.Upsert(ctx, c)
		if err != nil || s.Replaced {
			t.Fatalf("%s/%s: replaced=%v err=%v (kunci berbeda tidak boleh menimpa)", c.Provider, c.Host, s.Replaced, err)
		}
	}
	if n := rowCount(t, d, `true`); n != 4 {
		t.Fatalf("rows=%d", n)
	}
}

func TestRepo_UpsertConcurrentSingleRow(t *testing.T) {
	ctx := context.Background()
	d := setupDB(t)
	r := newRepo(d)
	u := mkUser(t, d, "a@tim.id")
	var wg sync.WaitGroup
	var mu sync.Mutex
	var inserted, replaced, failed int
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := r.Upsert(ctx, cred(u, base.ProviderGitLab, "gitlab.com", "ZZZZ"))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				failed++
			case s.Replaced:
				replaced++
			default:
				inserted++
			}
		}()
	}
	wg.Wait()
	if failed != 0 || inserted != 1 || replaced != 7 {
		t.Fatalf("inserted=%d replaced=%d failed=%d", inserted, replaced, failed)
	}
	if n := rowCount(t, d, `user_id=$1`, u); n != 1 {
		t.Fatalf("rows=%d", n)
	}
}

func TestRepo_UpsertConstraintMapping(t *testing.T) {
	ctx := context.Background()
	d := setupDB(t)
	r := newRepo(d)
	u := mkUser(t, d, "a@tim.id")

	// github dengan host selain github.com -> HOST_NOT_ALLOWED (constraint git_credentials_github_host).
	_, err := r.Upsert(ctx, cred(u, base.ProviderGitHub, "ghe.example.com", "AAAA"))
	if code(t, err) != "HOST_NOT_ALLOWED" {
		t.Fatalf("github host: %v", err)
	}
	// host huruf besar -> HOST_NOT_ALLOWED (git_credentials_host_check).
	_, err = r.Upsert(ctx, cred(u, base.ProviderGitLab, "GitLab.com", "AAAA"))
	if code(t, err) != "HOST_NOT_ALLOWED" {
		t.Fatalf("host check: %v", err)
	}
	// user tidak ada (FK) -> UNAUTHORIZED.
	_, err = r.Upsert(ctx, cred("0192f5a0-dead-7aaa-8bbb-00000000dead", base.ProviderGitLab, "gitlab.com", "AAAA"))
	if code(t, err) != "UNAUTHORIZED" {
		t.Fatalf("fk user: %v", err)
	}
	// bukan uuid -> unauthenticated tanpa menyentuh DB.
	if _, err = r.Upsert(ctx, cred("bukan-uuid", base.ProviderGitLab, "gitlab.com", "AAAA")); err == nil {
		t.Fatal("uuid tidak valid harus ditolak")
	}
	if n := rowCount(t, d, `true`); n != 0 {
		t.Fatalf("tidak boleh ada baris tersimpan: %d", n)
	}
}

// Canary: usecase penuh dengan Postgres nyata. Plaintext PAT tidak boleh muncul di kolom mana pun
// maupun di audit; token_enc harus bisa dibuka dengan AAD yang benar.
func TestExecute_EndToEnd_CanaryNotPersisted(t *testing.T) {
	d := setupDB(t)
	u := mkUser(t, d, "a@tim.id")
	box, err := secretbox.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	aud := &fakeAudit{}
	pol := fakePolicy{allow: map[string]bool{"github|github.com": true, "gitlab|gitlab.com": true}}
	uc := NewUsecase(true, pol, &fakeVerifier{}, box, newRepo(d), aud, base.SystemClock, nil)
	ctx := ctxFor(u)

	res, err := uc.Execute(ctx, "github", req("", canary), base.RequestMeta{})
	if err != nil || res.Replaced {
		t.Fatalf("simpan pertama: %+v %v", res, err)
	}
	res, err = uc.Execute(ctx, "github", req("", canary), base.RequestMeta{})
	if err != nil || !res.Replaced {
		t.Fatalf("simpan kedua harus replaced: %+v %v", res, err)
	}
	if len(aud.events) != 2 || aud.events[0].Args["replaced"] != false || aud.events[1].Args["replaced"] != true {
		t.Fatalf("audit: %+v", aud.events)
	}

	var enc []byte
	var rowText string
	if err := d.QueryRow(`SELECT token_enc, (git_credentials.*)::text FROM git_credentials WHERE user_id=$1`, u).Scan(&enc, &rowText); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(enc, []byte(canary)) || bytes.Contains([]byte(rowText), []byte(canary)) {
		t.Fatal("plaintext PAT tersimpan di database")
	}
	plain, err := box.Open(enc, base.AAD(u, base.ProviderGitHub, "github.com"))
	if err != nil || string(plain) != canary {
		t.Fatalf("open: %v %q", err, plain)
	}
	// AAD milik user/host lain tidak boleh bisa membuka.
	if _, err := box.Open(enc, base.AAD(u, base.ProviderGitLab, "gitlab.com")); err == nil {
		t.Fatal("AAD berbeda harus gagal")
	}
	for _, ev := range aud.events {
		if bytes.Contains([]byte(fmt.Sprint(ev.Args)), []byte(canary)) {
			t.Fatal("audit memuat PAT")
		}
	}
}
