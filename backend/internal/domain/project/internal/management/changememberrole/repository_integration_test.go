//go:build integration

package changememberrole

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
)

func migratedDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func seed(t *testing.T, d *db.DB, owners, members int) (pid string, ownerIDs, memberIDs []string) {
	t.Helper()
	ctx := context.Background()
	pid = uuid.NewString()
	if _, err := d.ExecContext(ctx, `INSERT INTO projects (id, name, description, created_at, updated_at) VALUES ($1,$2,'',now(),now())`, pid, "P-"+pid); err != nil {
		t.Fatal(err)
	}
	add := func(role string) string {
		id := uuid.NewString()
		if _, err := d.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)`, pid, id, role); err != nil {
			t.Fatal(err)
		}
		return id
	}
	for i := 0; i < owners; i++ {
		ownerIDs = append(ownerIDs, add(base.RoleOwner))
	}
	for i := 0; i < members; i++ {
		memberIDs = append(memberIDs, add(base.RoleMember))
	}
	return
}

func roleOf(t *testing.T, d *db.DB, pid, uid string) string {
	t.Helper()
	var r string
	if err := d.GetContext(context.Background(), &r, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, pid, uid); err != nil {
		t.Fatal(err)
	}
	return r
}

func countOwners(t *testing.T, d *db.DB, pid string) int {
	t.Helper()
	var n int
	if err := d.GetContext(context.Background(), &n, `SELECT count(*) FROM project_members WHERE project_id=$1 AND role='owner'`, pid); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepo_UpdateMemberRole(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	pid, _, ms := seed(t, d, 1, 1)
	var before time.Time
	if err := d.GetContext(ctx, &before, `SELECT updated_at FROM projects WHERE id=$1`, pid); err != nil {
		t.Fatal(err)
	}
	err := r.InTx(ctx, func(tx TxRepository) error { return tx.UpdateMemberRole(ctx, pid, ms[0], base.RoleOwner) })
	if err != nil || roleOf(t, d, pid, ms[0]) != base.RoleOwner {
		t.Fatalf("%v", err)
	}
	var after time.Time
	if err := d.GetContext(ctx, &after, `SELECT updated_at FROM projects WHERE id=$1`, pid); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatal("projects.updated_at berubah")
	}
	err = r.InTx(ctx, func(tx TxRepository) error { return tx.UpdateMemberRole(ctx, pid, uuid.NewString(), base.RoleMember) })
	if !errors.Is(err, base.ErrMemberNotFound) {
		t.Fatalf("want ErrMemberNotFound, got %v", err)
	}
	err = r.InTx(ctx, func(tx TxRepository) error { return tx.UpdateMemberRole(ctx, pid, ms[0], "admin") })
	if err == nil || errors.Is(err, base.ErrMemberNotFound) {
		t.Fatalf("want CHECK error, got %v", err)
	}
}

func TestRepo_InTxRollback(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	pid, _, ms := seed(t, d, 1, 1)
	boom := errors.New("boom")
	err := r.InTx(ctx, func(tx TxRepository) error {
		if err := tx.UpdateMemberRole(ctx, pid, ms[0], base.RoleOwner); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || roleOf(t, d, pid, ms[0]) != base.RoleMember {
		t.Fatalf("rollback gagal: %v", err)
	}
}

func TestRepo_LockSerializesTransactions(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d, store.New(d))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pid, _, ms := seed(t, d, 1, 1)

	locked := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = r.InTx(ctx, func(tx TxRepository) error {
			if _, _, err := tx.LockProject(ctx, pid); err != nil {
				return err
			}
			if _, err := tx.LockMembers(ctx, pid); err != nil {
				return err
			}
			close(locked)
			<-release
			return tx.UpdateMemberRole(ctx, pid, ms[0], base.RoleOwner)
		})
	}()
	<-locked

	var seen string
	done := make(chan error, 1)
	go func() {
		done <- r.InTx(ctx, func(tx TxRepository) error {
			if _, _, err := tx.LockProject(ctx, pid); err != nil {
				return err
			}
			list, err := tx.LockMembers(ctx, pid)
			if err != nil {
				return err
			}
			m, _ := base.FindMember(list, ms[0])
			seen = m.Role
			return nil
		})
	}()
	select {
	case <-done:
		t.Fatal("tx B tidak menunggu tx A")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if seen != base.RoleOwner {
		t.Fatalf("tx B melihat role %q, want owner", seen)
	}
}

type itUsers struct{}

func (itUsers) Get(_ context.Context, id string) (getuser.Response, error) {
	return getuser.Response{Found: true, ID: id, Email: id + "@tim.id", Role: "developer", IsActive: true}, nil
}
func (itUsers) FindDevelopers(context.Context, string, int) ([]finddevelopers.Developer, error) {
	return nil, nil
}

type itAudit struct {
	mu sync.Mutex
	n  int
}

func (a *itAudit) Record(context.Context, audit.Event) { a.mu.Lock(); a.n++; a.mu.Unlock() }

func asUser(id string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Email: id + "@tim.id", Role: "developer"})
}

// race dua owner saling menurunkan: tepat satu sukses, satu 403, sisa 1 owner, 1 audit.
func TestT03_AC3_ConcurrentMutualDemotion(t *testing.T) {
	d := migratedDB(t)
	for i := 0; i < 20; i++ {
		pid, os, _ := seed(t, d, 2, 1)
		a, c := os[0], os[1]
		au := &itAudit{}
		uc := NewUsecase(newRepo(d, store.New(d)), itUsers{}, au, slog.Default())
		start := make(chan struct{})
		errsC := make([]error, 2)
		var wg sync.WaitGroup
		for k, p := range [][2]string{{a, c}, {c, a}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errsC[k] = uc.Execute(asUser(p[0]), Input{ProjectID: pid, UserID: p[1], Role: base.RoleMember})
			}()
		}
		close(start)
		wg.Wait()
		ok, forbidden := 0, 0
		for _, e := range errsC {
			switch {
			case e == nil:
				ok++
			case errors.Is(e, base.ErrForbidden):
				forbidden++
			default:
				t.Fatalf("round %d: error tak terduga %v", i, e)
			}
		}
		if ok != 1 || forbidden != 1 || countOwners(t, d, pid) != 1 || au.n != 1 {
			t.Fatalf("round %d: ok=%d forbidden=%d owners=%d audit=%d", i, ok, forbidden, countOwners(t, d, pid), au.n)
		}
	}
}

// dua owner menurunkan dirinya sendiri: satu 200, satu 409 LAST_OWNER.
func TestT03_AC3_ConcurrentSelfDemotion(t *testing.T) {
	d := migratedDB(t)
	for i := 0; i < 20; i++ {
		pid, os, _ := seed(t, d, 2, 0)
		au := &itAudit{}
		uc := NewUsecase(newRepo(d, store.New(d)), itUsers{}, au, slog.Default())
		start := make(chan struct{})
		errsC := make([]error, 2)
		var wg sync.WaitGroup
		for k, id := range os {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errsC[k] = uc.Execute(asUser(id), Input{ProjectID: pid, UserID: id, Role: base.RoleMember})
			}()
		}
		close(start)
		wg.Wait()
		ok, last := 0, 0
		for _, e := range errsC {
			switch {
			case e == nil:
				ok++
			case errors.Is(e, base.ErrLastOwner):
				last++
			default:
				t.Fatalf("round %d: error tak terduga %v", i, e)
			}
		}
		if ok != 1 || last != 1 || countOwners(t, d, pid) != 1 || au.n != 1 {
			t.Fatalf("round %d: ok=%d last=%d owners=%d audit=%d", i, ok, last, countOwners(t, d, pid), au.n)
		}
	}
}
