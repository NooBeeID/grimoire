//go:build integration

package removemember

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

func seedProject(t *testing.T, d *db.DB) (pid, owner, member string) {
	t.Helper()
	ctx := context.Background()
	owner, member = uuid.NewString(), uuid.NewString()
	if err := d.GetContext(ctx, &pid, `INSERT INTO projects (name) VALUES ('Alpha') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,'owner'),($1,$3,'member')`, pid, owner, member); err != nil {
		t.Fatal(err)
	}
	return
}

func countMembers(t *testing.T, d *db.DB, pid string) int {
	t.Helper()
	var n int
	if err := d.GetContext(context.Background(), &n, `SELECT count(*) FROM project_members WHERE project_id = $1`, pid); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepo_DeleteMember(t *testing.T) {
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	pid, owner, member := seedProject(t, d)

	err := r.InTx(ctx, func(tx TxRepository) error { return tx.DeleteMember(ctx, pid, member) })
	if err != nil || countMembers(t, d, pid) != 1 {
		t.Fatalf("%v %d", err, countMembers(t, d, pid))
	}
	_, _, ok, _ := r.FindAccess(ctx, pid, owner)
	if !ok {
		t.Fatal("project/owner hilang")
	}
	err = r.InTx(ctx, func(tx TxRepository) error { return tx.DeleteMember(ctx, pid, member) })
	if !errors.Is(err, base.ErrMemberNotFound) {
		t.Fatal(err)
	}
}

func TestRepo_InTxRollback(t *testing.T) {
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	pid, _, member := seedProject(t, d)
	boom := errors.New("boom")
	err := r.InTx(ctx, func(tx TxRepository) error {
		if err := tx.DeleteMember(ctx, pid, member); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || countMembers(t, d, pid) != 2 {
		t.Fatalf("%v %d", err, countMembers(t, d, pid))
	}
}

func TestRepo_DeleteMemberInjectionSafe(t *testing.T) {
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	pid, _, _ := seedProject(t, d)
	_ = r.InTx(ctx, func(tx TxRepository) error {
		return tx.DeleteMember(ctx, pid, `'); DROP TABLE project_members;--`)
	})
	if countMembers(t, d, pid) != 2 {
		t.Fatal("tabel terpengaruh")
	}
}
