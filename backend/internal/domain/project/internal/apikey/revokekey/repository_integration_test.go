//go:build integration

package revokekey

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/audit"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
)

type ienv struct {
	d   *db.DB
	uc  *Usecase
	acc *fakeAccess
	aud *syncAudit
}

type syncAudit struct {
	n atomic.Int32
}

func (s *syncAudit) Record(context.Context, audit.Event) { s.n.Add(1) }

func isetup(t *testing.T) ienv {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	acc := &fakeAccess{l: &log{}}
	aud := &syncAudit{}
	uc := NewUsecase(newRepo(d, store.New(d)), acc, aud, base.SystemClock)
	return ienv{d: d, uc: uc, acc: acc, aud: aud}
}

func (e ienv) project(t *testing.T, name string) string {
	t.Helper()
	var id string
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

var iseq int

const (
	userA = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"
	userB = "0192f5a0-bbbb-7aaa-8bbb-00000000000b"
	userS = "0192f5a0-cccc-7aaa-8bbb-00000000000c"
)

// key membuat key milik owner ("" -> legacy, created_by NULL).
func (e ienv) key(t *testing.T, pid, owner, name string) string {
	t.Helper()
	iseq++
	h := sha256.Sum256([]byte("s"))
	var id string
	var by any
	if owner != "" {
		by = owner
	}
	err := e.d.GetContext(context.Background(), &id,
		`INSERT INTO api_keys (project_id, name, prefix, secret_hash, created_by) VALUES ($1,$2,$3,$4,$5::uuid) RETURNING id::text`,
		pid, name+" key", fmt.Sprintf("rk%06d", iseq), h[:], by)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e ienv) row(t *testing.T, id string) base.Key {
	t.Helper()
	k, found, err := store.New(e.d).GetKey(context.Background(), `SELECT `+store.KeyColumns+` FROM api_keys WHERE id = $1`, id)
	if err != nil || !found {
		t.Fatalf("row: %v %v", found, err)
	}
	return k
}

func ictx(id, email, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Email: email, Role: role})
}

func uctx() context.Context  { return ictx(userA, "a@tim.id", "developer") }
func bctx() context.Context  { return ictx(userB, "b@tim.id", "developer") }
func suctx() context.Context { return ictx(userS, "admin@tim.id", "super_admin") }

func TestT04_Revoke_SuccessThenConflict(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	k := e.key(t, p, userA, "ci")
	dto, err := e.uc.Execute(uctx(), Input{ProjectID: p, KeyID: k})
	if err != nil || string(dto.Status) != "revoked" {
		t.Fatalf("dto=%+v err=%v", dto, err)
	}
	r1 := e.row(t, k)
	if r1.RevokedAt == nil || r1.RevokedBy == nil || *r1.RevokedBy != userA || *r1.RevokedByLabel != "a@tim.id" || r1.LastUsedAt != nil {
		t.Fatalf("row=%+v", r1)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := e.uc.Execute(uctx(), Input{ProjectID: p, KeyID: k}); !errors.Is(err, base.ErrAlreadyRevoked) {
		t.Fatalf("err=%v", err)
	}
	r2 := e.row(t, k)
	if !r2.RevokedAt.Equal(*r1.RevokedAt) || *r2.RevokedBy != *r1.RevokedBy {
		t.Fatal("revoke kedua mengubah baris")
	}
	if e.aud.n.Load() != 1 {
		t.Fatalf("audit=%d", e.aud.n.Load())
	}
}

func TestT04_Security_ConcurrentRevokeExactlyOne(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	for round := 0; round < 5; round++ {
		k := e.key(t, p, userA, fmt.Sprintf("k%d", round))
		before := e.aud.n.Load()
		var ok, conflict atomic.Int32
		var wg sync.WaitGroup
		gate := make(chan struct{})
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				_, err := e.uc.Execute(uctx(), Input{ProjectID: p, KeyID: k})
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, base.ErrAlreadyRevoked):
					conflict.Add(1)
				default:
					t.Errorf("err=%v", err)
				}
			}()
		}
		close(gate)
		wg.Wait()
		if ok.Load() != 1 || conflict.Load() != 9 || e.aud.n.Load()-before != 1 {
			t.Fatalf("ok=%d conflict=%d audit=%d", ok.Load(), conflict.Load(), e.aud.n.Load()-before)
		}
	}
}

func TestT04_Security_ProjectIsolation(t *testing.T) {
	e := isetup(t)
	p1, p2 := e.project(t, "P1"), e.project(t, "P2")
	k1, k2 := e.key(t, p1, userA, "a"), e.key(t, p2, userA, "b")
	for _, c := range [][2]string{{p1, k2}, {p2, k1}} {
		if _, err := e.uc.Execute(uctx(), Input{ProjectID: c[0], KeyID: c[1]}); !errors.Is(err, base.ErrKeyNotFound) {
			t.Fatalf("err=%v", err)
		}
	}
	if e.row(t, k1).RevokedAt != nil || e.row(t, k2).RevokedAt != nil || e.aud.n.Load() != 0 {
		t.Fatal("key berubah atau audit tercatat")
	}
}

func TestT04_Revoke_NonUUIDKeyID(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	for _, id := range []string{"bukan-uuid", "0192f5a0-2222-7aaa-8bbb-000000000001"} {
		if _, err := e.uc.Execute(uctx(), Input{ProjectID: p, KeyID: id}); !errors.Is(err, base.ErrKeyNotFound) {
			t.Fatalf("%s err=%v", id, err)
		}
	}
}

func TestT04_Revoke_NonMemberGets404(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	k := e.key(t, p, userA, "ci")
	e.acc.err = base.ErrProjectNotFound
	if _, err := e.uc.Execute(uctx(), Input{ProjectID: p, KeyID: k}); !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("err=%v", err)
	}
	if e.row(t, k).RevokedAt != nil || e.aud.n.Load() != 0 {
		t.Fatal("key berubah")
	}
}

func TestT04_AC4_RevokedNameReusable(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	k := e.key(t, p, userA, "ci")
	if _, err := e.uc.Execute(uctx(), Input{ProjectID: p, KeyID: k}); err != nil {
		t.Fatal(err)
	}
	e.key(t, p, userA, "ci") // nama bebas setelah dicabut
}

func TestT13_CrossUser_NotFound_RowUntouched(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	k := e.key(t, p, userA, "a")
	if _, err := e.uc.Execute(bctx(), Input{ProjectID: p, KeyID: k}); !errors.Is(err, base.ErrKeyNotFound) {
		t.Fatalf("err=%v", err)
	}
	r := e.row(t, k)
	if r.RevokedAt != nil || r.RevokedBy != nil || e.aud.n.Load() != 0 {
		t.Fatalf("row=%+v audit=%d", r, e.aud.n.Load())
	}
}

func TestT13_Lock_ScopeB_OnKeyA_NotBlocked(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	k := e.key(t, p, userA, "a")
	ctx := context.Background()
	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- newRepo(e.d, store.New(e.d)).InTx(ctx, func(tx txRepo) error {
			if _, found, err := tx.LockInProject(ctx, p, k, base.OwnerScope{OwnerID: userA}); err != nil || !found {
				return fmt.Errorf("lock A: %v %v", found, err)
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	res := make(chan bool, 1)
	go func() {
		_ = newRepo(e.d, store.New(e.d)).InTx(ctx, func(tx txRepo) error {
			_, found, err := tx.LockInProject(ctx, p, k, base.OwnerScope{OwnerID: userB})
			res <- found || err != nil
			return nil
		})
	}()
	select {
	case bad := <-res:
		if bad {
			t.Fatal("B menemukan key A")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("B terblokir oleh lock A")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if e.row(t, k).RevokedAt != nil {
		t.Fatal("baris berubah")
	}
}

func TestT13_Legacy_SuperAdminOnly(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	k := e.key(t, p, "", "legacy")
	ctx := context.Background()
	r := newRepo(e.d, store.New(e.d))
	_ = r.InTx(ctx, func(tx txRepo) error {
		if _, f, err := tx.LockInProject(ctx, p, k, base.OwnerScope{All: true}); err != nil || !f {
			t.Fatalf("all: %v %v", f, err)
		}
		if _, f, err := tx.LockInProject(ctx, p, k, base.OwnerScope{OwnerID: userA}); err != nil || f {
			t.Fatalf("owner: %v %v", f, err)
		}
		return nil
	})
	if _, err := e.uc.Execute(uctx(), Input{ProjectID: p, KeyID: k}); !errors.Is(err, base.ErrKeyNotFound) {
		t.Fatalf("err=%v", err)
	}
	if _, err := e.uc.Execute(suctx(), Input{ProjectID: p, KeyID: k}); err != nil {
		t.Fatal(err)
	}
}

func TestT13_SuperAdmin_RevokesOthers(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	k := e.key(t, p, userB, "b")
	if _, err := e.uc.Execute(suctx(), Input{ProjectID: p, KeyID: k}); err != nil {
		t.Fatal(err)
	}
	r := e.row(t, k)
	if r.RevokedBy == nil || *r.RevokedBy != userS || r.CreatedBy == nil || *r.CreatedBy != userB || e.aud.n.Load() != 1 {
		t.Fatalf("row=%+v", r)
	}
}

func TestT13_Lock_FailClosed(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	k := e.key(t, p, userA, "a")
	ctx := context.Background()
	_ = newRepo(e.d, store.New(e.d)).InTx(ctx, func(tx txRepo) error {
		cases := []struct {
			p, k string
			s    base.OwnerScope
		}{
			{p, k, base.OwnerScope{}},
			{p, k, base.OwnerScope{OwnerID: "bukan-uuid"}},
			{p, "bukan-uuid", base.OwnerScope{All: true}},
		}
		for i, c := range cases {
			if _, f, err := tx.LockInProject(ctx, c.p, c.k, c.s); f || err != nil {
				t.Fatalf("case %d: %v %v", i, f, err)
			}
		}
		return nil
	})
}

func TestT13_ProjectIsolation_SuperAdminToo(t *testing.T) {
	e := isetup(t)
	p1, p2 := e.project(t, "P1"), e.project(t, "P2")
	k := e.key(t, p1, userA, "a")
	for _, c := range []context.Context{uctx(), suctx()} {
		if _, err := e.uc.Execute(c, Input{ProjectID: p2, KeyID: k}); !errors.Is(err, base.ErrKeyNotFound) {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestT13_Concurrent_OtherUserCannotBlock(t *testing.T) {
	e := isetup(t)
	p := e.project(t, "P1")
	for round := 0; round < 5; round++ {
		k := e.key(t, p, userA, fmt.Sprintf("c%d", round))
		before := e.aud.n.Load()
		var wg sync.WaitGroup
		gate := make(chan struct{})
		var errA, errB error
		wg.Add(2)
		go func() { defer wg.Done(); <-gate; _, errA = e.uc.Execute(uctx(), Input{ProjectID: p, KeyID: k}) }()
		go func() { defer wg.Done(); <-gate; _, errB = e.uc.Execute(bctx(), Input{ProjectID: p, KeyID: k}) }()
		close(gate)
		wg.Wait()
		if errA != nil || !errors.Is(errB, base.ErrKeyNotFound) || e.aud.n.Load()-before != 1 {
			t.Fatalf("errA=%v errB=%v audit=%d", errA, errB, e.aud.n.Load()-before)
		}
	}
}
