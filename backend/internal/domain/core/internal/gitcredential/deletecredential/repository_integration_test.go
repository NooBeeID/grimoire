//go:build integration

package deletecredential

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/db"
)

const (
	userA = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"
	userB = "0192f5a0-bbbb-7aaa-8bbb-00000000000b"
)

func setup(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, slog.Default()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for i, u := range []string{userA, userB} {
		if _, err := d.Exec(`INSERT INTO users (id,email,name,password_hash,role) VALUES ($1,$2,'U','x','developer')`,
			u, []string{"a@x.id", "b@x.id"}[i]); err != nil {
			t.Fatalf("user: %v", err)
		}
	}
	return d
}

func cred(t *testing.T, d *db.DB, user, provider, host string) {
	t.Helper()
	if _, err := d.Exec(`INSERT INTO git_credentials (user_id,provider,host,token_enc,last4) VALUES ($1,$2,$3,decode(repeat('ab',40),'hex'),'7Qx2')`,
		user, provider, host); err != nil {
		t.Fatalf("cred: %v", err)
	}
}

func count(t *testing.T, d *db.DB, where string, args ...any) int {
	t.Helper()
	var n int
	if err := d.Get(&n, `SELECT count(*) FROM git_credentials WHERE `+where, args...); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepo_OwnRowAndRepeat(t *testing.T) {
	d := setup(t)
	cred(t, d, userA, "github", "github.com")
	r := newRepo(d)
	if ok, err := r.Delete(context.Background(), userA, base.ProviderGitHub, "github.com"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := r.Delete(context.Background(), userA, base.ProviderGitHub, "github.com"); ok || err != nil {
		t.Fatal(ok, err)
	}
}

func TestRepo_OtherUserUntouched(t *testing.T) {
	d := setup(t)
	cred(t, d, userA, "github", "github.com")
	cred(t, d, userB, "github", "github.com")
	r := newRepo(d)
	if ok, _ := r.Delete(context.Background(), userA, base.ProviderGitHub, "github.com"); !ok {
		t.Fatal("harus terhapus")
	}
	if count(t, d, `user_id=$1`, userB) != 1 {
		t.Fatal("baris B hilang")
	}
	// B menghapus baris yang hanya milik A -> false
	cred(t, d, userA, "gitlab", "gitlab.com")
	if ok, _ := r.Delete(context.Background(), userB, base.ProviderGitLab, "gitlab.com"); ok {
		t.Fatal("tidak boleh menghapus baris A")
	}
	if count(t, d, `user_id=$1`, userA) != 1 {
		t.Fatal("baris A hilang")
	}
}

func TestRepo_OtherHostAndProvider(t *testing.T) {
	d := setup(t)
	cred(t, d, userA, "gitlab", "gitlab.com")
	cred(t, d, userA, "gitlab", "git.noobee.id")
	cred(t, d, userA, "github", "github.com")
	r := newRepo(d)
	ctx := context.Background()
	if ok, _ := r.Delete(ctx, userA, base.ProviderGitLab, "git.noobee.id"); !ok {
		t.Fatal("harus terhapus")
	}
	if count(t, d, `host='gitlab.com'`) != 1 {
		t.Fatal("gitlab.com hilang")
	}
	if ok, _ := r.Delete(ctx, userA, base.ProviderGitLab, "git.lain.id"); ok {
		t.Fatal("host tak ada")
	}
	if ok, _ := r.Delete(ctx, userA, base.ProviderGitLab, "github.com"); ok {
		t.Fatal("provider salah")
	}
	if count(t, d, `user_id=$1`, userA) != 2 {
		t.Fatal("jumlah baris berubah")
	}
}

func TestRepo_NonUUIDUser(t *testing.T) {
	d := setup(t)
	if ok, err := newRepo(d).Delete(context.Background(), "bukan-uuid", base.ProviderGitHub, "github.com"); ok || err != nil {
		t.Fatal(ok, err)
	}
}

func TestRepo_ConcurrentExactlyOne(t *testing.T) {
	d := setup(t)
	cred(t, d, userA, "github", "github.com")
	r := newRepo(d)
	var wins, errsN atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := r.Delete(context.Background(), userA, base.ProviderGitHub, "github.com")
			if err != nil {
				errsN.Add(1)
			}
			if ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 || errsN.Load() != 0 {
		t.Fatalf("wins %d errs %d", wins.Load(), errsN.Load())
	}
}

func TestExecute_EndToEnd(t *testing.T) {
	d := setup(t)
	cred(t, d, userA, "gitlab", "git.dikeluarkan.id")
	a := &fakeAudit{}
	uc := NewUsecase(newRepo(d), a)
	ctx := ctxFor(userA)
	if err := uc.Execute(ctx, Input{Provider: "gitlab", Host: "git.dikeluarkan.id"}); err != nil {
		t.Fatal(err)
	}
	if len(a.events) != 1 || count(t, d, `user_id=$1 AND provider='gitlab'`, userA) != 0 {
		t.Fatal("hasil tidak sesuai")
	}
	cred(t, d, userA, "github", "github.com")
	if err := uc.Execute(ctxFor(userB), Input{Provider: "github"}); err == nil {
		t.Fatal("B harus 404")
	}
	if len(a.events) != 1 || count(t, d, `user_id=$1`, userA) != 1 {
		t.Fatal("audit/baris A berubah")
	}
}
