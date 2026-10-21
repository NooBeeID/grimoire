//go:build integration

package addmember

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

func migratedDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func seedProject(t *testing.T, d *db.DB) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := d.ExecContext(context.Background(),
		`INSERT INTO projects (id, name, description, created_at, updated_at) VALUES ($1, $2, '', now(), now())`,
		id, "P-"+id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newMember(t *testing.T, pid, role string) base.Member {
	t.Helper()
	a := base.Actor{ID: uuid.NewString(), Role: "developer", Label: "a@tim.id"}
	m, err := base.NewMember(pid, uuid.NewString(), role, a, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRepo_InsertMember_AndConflict(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	pid := seedProject(t, d)
	m := newMember(t, pid, base.RoleMember)
	var first, second bool
	err := r.InTx(ctx, func(tx Tx) (err error) {
		if first, err = tx.InsertMember(ctx, m); err != nil {
			return err
		}
		second, err = tx.InsertMember(ctx, m)
		return err
	})
	if err != nil || !first || second {
		t.Fatalf("%v %v %v", err, first, second)
	}
	var n int
	if err := d.GetContext(ctx, &n, `SELECT count(*) FROM project_members WHERE project_id=$1`, pid); err != nil || n != 1 {
		t.Fatalf("%v %d", err, n)
	}
	var label string
	if err := d.GetContext(ctx, &label, `SELECT added_by_label FROM project_members WHERE project_id=$1`, pid); err != nil || label != "a@tim.id" {
		t.Fatalf("%v %q", err, label)
	}
}

func TestRepo_InsertMember_SafetyNets(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	pid := seedProject(t, d)

	bad := newMember(t, pid, base.RoleMember)
	bad.Role = "admin"
	if err := r.InTx(ctx, func(tx Tx) error { _, err := tx.InsertMember(ctx, bad); return err }); err == nil {
		t.Fatal("CHECK role harus gagal")
	}

	orphan := newMember(t, uuid.NewString(), base.RoleMember)
	if err := r.InTx(ctx, func(tx Tx) error { _, err := tx.InsertMember(ctx, orphan); return err }); err == nil {
		t.Fatal("FK project harus gagal")
	}
}

func TestRepo_LockProjectBlocksSecondTx(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	pid := seedProject(t, d)
	m := newMember(t, pid, base.RoleMember)

	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- r.InTx(ctx, func(tx Tx) error {
			if _, _, err := tx.LockProject(ctx, pid); err != nil {
				return err
			}
			close(locked)
			<-release
			_, err := tx.InsertMember(ctx, m)
			return err
		})
	}()
	<-locked
	res := make(chan error, 1)
	go func() {
		res <- r.InTx(ctx, func(tx Tx) error {
			if _, _, err := tx.LockProject(ctx, pid); err != nil {
				return err
			}
			ms, err := tx.LockMembers(ctx, pid)
			if err != nil {
				return err
			}
			if _, ok := base.FindMember(ms, m.UserID); ok {
				return base.ErrAlreadyMember
			}
			return nil
		})
	}()
	select {
	case <-res:
		t.Fatal("tx kedua tidak boleh selesai sebelum commit tx pertama")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-res; err != base.ErrAlreadyMember {
		t.Fatalf("want ErrAlreadyMember, got %v", err)
	}
}
