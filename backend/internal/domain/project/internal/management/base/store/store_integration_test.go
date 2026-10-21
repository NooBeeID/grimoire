//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/infra/db"
)

func seed(t *testing.T, d *db.DB) (pid, owner, member string) {
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

func TestStore_FindAccess(t *testing.T) {
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := New(d)
	ctx := context.Background()
	pid, owner, member := seed(t, d)

	if _, role, ok, err := s.FindAccess(ctx, pid, owner); err != nil || !ok || role != "owner" {
		t.Fatalf("owner: %q %v %v", role, ok, err)
	}
	if _, role, ok, _ := s.FindAccess(ctx, pid, member); !ok || role != "member" {
		t.Fatalf("member: %q %v", role, ok)
	}
	if _, role, ok, _ := s.FindAccess(ctx, pid, uuid.NewString()); !ok || role != "" {
		t.Fatalf("bukan anggota: %q %v", role, ok)
	}
	if _, role, ok, _ := s.FindAccess(ctx, pid, "bukan-uuid"); !ok || role != "" {
		t.Fatalf("user bukan uuid: %q %v", role, ok)
	}
	if _, _, ok, err := s.FindAccess(ctx, uuid.NewString(), owner); ok || err != nil {
		t.Fatalf("tidak ada: %v %v", ok, err)
	}
	if _, _, ok, err := s.FindAccess(ctx, "bukan-uuid", owner); ok || err != nil {
		t.Fatalf("project bukan uuid: %v %v", ok, err)
	}
}

func TestStore_FindProjectListMembersCascade(t *testing.T) {
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := New(d)
	ctx := context.Background()
	pid, _, _ := seed(t, d)

	p, ok, err := s.FindProject(ctx, pid)
	if err != nil || !ok || p.Name != "Alpha" {
		t.Fatalf("%+v %v %v", p, ok, err)
	}
	if _, ok, err := s.FindProject(ctx, "x"); ok || err != nil {
		t.Fatal("bukan uuid")
	}
	ms, err := s.ListMembers(ctx, pid)
	if err != nil || len(ms) != 2 || ms[0].UserID > ms[1].UserID {
		t.Fatalf("%+v %v", ms, err)
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, pid); err != nil {
		t.Fatal(err)
	}
	if ms, _ := s.ListMembers(ctx, pid); len(ms) != 0 {
		t.Fatal("cascade gagal")
	}
}

func TestStore_LockProjectBlocksSecondTx(t *testing.T) {
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	pid, _, _ := seed(t, d)

	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan string, 1)
	go func() {
		_ = d.BeginTx(ctx, func(tx *sqlx.Tx) error {
			st := New(d).WithTx(tx)
			if _, _, err := st.LockProject(ctx, pid); err != nil {
				t.Error(err)
			}
			close(locked)
			<-release
			_, err := tx.ExecContext(ctx, `UPDATE projects SET name = 'Beta' WHERE id = $1`, pid)
			return err
		})
	}()
	<-locked
	go func() {
		_ = d.BeginTx(ctx, func(tx *sqlx.Tx) error {
			p, _, err := New(d).WithTx(tx).LockProject(ctx, pid)
			if err != nil {
				t.Error(err)
			}
			done <- p.Name
			return nil
		})
	}()
	select {
	case n := <-done:
		t.Fatalf("tx kedua tidak terblokir (%s)", n)
	default:
	}
	close(release)
	if n := <-done; n != "Beta" {
		t.Fatalf("tx kedua melihat %q, want Beta", n)
	}
}
