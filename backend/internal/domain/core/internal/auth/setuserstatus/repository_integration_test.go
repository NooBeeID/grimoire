//go:build integration

package setuserstatus

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/migrations"
)

func setupRepo(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, NewRepo(d)
}

func seed(t *testing.T, d *db.DB, role string, active bool) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, is_active)
		VALUES ('s-'||gen_random_uuid()::text||'@x.id', 'N', 'h', $1, $2) RETURNING id::text`, role, active); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedSession(t *testing.T, d *db.DB, uid, h string) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), `INSERT INTO sessions (user_id, token_hash, last_seen_at, expires_at)
		VALUES ($1, $2, now(), now() + interval '8 hours')`, uid, []byte(h)); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, d *db.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.GetContext(context.Background(), &n, q, args...); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepo_InTx_DeactivateDeletesSessionsAtomically(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	uid := seed(t, d, "developer", true)
	for _, h := range []string{"a", "b", "c"} {
		seedSession(t, d, uid, h)
	}
	boom := errors.New("boom")
	err := r.InTx(ctx, func(tx Tx) error {
		if err := tx.SetActive(ctx, uid, false, time.Now()); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || count(t, d, `SELECT count(*) FROM users WHERE id=$1 AND is_active`, uid) != 1 ||
		count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, uid) != 3 {
		t.Fatalf("rollback gagal: %v", err)
	}
	var n int64
	err = r.InTx(ctx, func(tx Tx) error {
		if err := tx.SetActive(ctx, uid, false, time.Now()); err != nil {
			return err
		}
		var e error
		n, e = tx.DeleteSessions(ctx, uid)
		return e
	})
	if err != nil || n != 3 || count(t, d, `SELECT count(*) FROM users WHERE id=$1 AND NOT is_active`, uid) != 1 ||
		count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, uid) != 0 {
		t.Fatalf("commit: n=%d err=%v", n, err)
	}
}

func TestRepo_LockActiveSuperAdminIDs_OrderedByID(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		seed(t, d, "super_admin", true)
	}
	seed(t, d, "super_admin", false)
	seed(t, d, "developer", true)
	var ids []string
	if err := r.InTx(ctx, func(tx Tx) error {
		var e error
		ids, e = tx.LockActiveSuperAdminIDs(ctx)
		return e
	}); err != nil || len(ids) != 4 {
		t.Fatalf("%v %v", ids, err)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			t.Fatalf("tidak terurut: %v", ids)
		}
	}
}

func TestRepo_NoDeadlock_MutualDeactivate(t *testing.T) {
	d, r := setupRepo(t)
	uc := NewUsecase(r, &fakeRec{}, base.SystemClock)
	a, b := seed(t, d, "super_admin", true), seed(t, d, "super_admin", true)
	for i := 0; i < 20; i++ {
		ctx := context.Background()
		if _, err := d.ExecContext(ctx, `UPDATE users SET is_active=true WHERE id IN ($1,$2)`, a, b); err != nil {
			t.Fatal(err)
		}
		errsC := make([]error, 2)
		var wg sync.WaitGroup
		run := func(i int, actor, target string) {
			defer wg.Done()
			c := reqctx.WithIdentity(ctx, reqctx.Identity{UserID: actor, Role: base.RoleSuperAdmin})
			_, errsC[i] = uc.Execute(c, Input{TargetID: target})
		}
		wg.Add(2)
		go run(0, a, b)
		go run(1, b, a)
		wg.Wait()
		ok, last := 0, 0
		for _, e := range errsC {
			switch {
			case e == nil:
				ok++
			case errors.Is(e, base.ErrLastSuperAdmin):
				last++
			default:
				t.Fatalf("iter %d: error tak terduga: %v", i, e)
			}
		}
		if ok != 1 || last != 1 || count(t, d, `SELECT count(*) FROM users WHERE role='super_admin' AND is_active`) != 1 {
			t.Fatalf("iter %d: ok=%d last=%d", i, ok, last)
		}
	}
}

func TestRepo_ActivateDoesNotRestoreSessions(t *testing.T) {
	d, r := setupRepo(t)
	ctx := context.Background()
	uid := seed(t, d, "developer", false)
	if err := r.InTx(ctx, func(tx Tx) error { return tx.SetActive(ctx, uid, true, time.Now()) }); err != nil {
		t.Fatal(err)
	}
	if count(t, d, `SELECT count(*) FROM sessions WHERE user_id=$1`, uid) != 0 {
		t.Fatal("sesi muncul")
	}
}
