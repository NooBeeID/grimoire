//go:build integration

package createproject

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/db"
)

func actor() base.Actor {
	return base.Actor{ID: uuid.NewString(), Role: "developer", Label: "a@tim.id"}
}

func TestRepo_InTxSuccess(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d)
	ctx := context.Background()
	a := actor()
	now := time.Now().UTC()
	p, _ := base.NewProject("Payments", "d", a, now)
	var created base.Project
	err := r.InTx(ctx, func(tx Tx) error {
		var err error
		if created, err = tx.InsertProject(ctx, p); err != nil {
			return err
		}
		m, _ := base.NewMember(created.ID, a.ID, base.RoleOwner, a, now)
		return tx.InsertMember(ctx, m)
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("created: %+v", created)
	}
	var label string
	if err := d.GetContext(ctx, &label, `SELECT created_by_label FROM projects WHERE id=$1`, created.ID); err != nil || label != "a@tim.id" {
		t.Fatalf("label %q %v", label, err)
	}
	var n int
	_ = d.GetContext(ctx, &n, `SELECT count(*) FROM project_members WHERE project_id=$1 AND role='owner'`, created.ID)
	if n != 1 {
		t.Fatalf("members %d", n)
	}
}

func TestRepo_AtomicRollback(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d)
	ctx := context.Background()
	a := actor()
	now := time.Now().UTC()
	p, _ := base.NewProject("Rollback", "", a, now)
	err := r.InTx(ctx, func(tx Tx) error {
		c, err := tx.InsertProject(ctx, p)
		if err != nil {
			return err
		}
		return tx.InsertMember(ctx, base.Member{ProjectID: c.ID, UserID: a.ID, Role: "admin", AddedAt: now})
	})
	if err == nil {
		t.Fatal("harus gagal (CHECK role)")
	}
	var n int
	_ = d.GetContext(ctx, &n, `SELECT count(*) FROM projects WHERE name='Rollback'`)
	if n != 0 {
		t.Fatalf("projects tersisa %d", n)
	}
}

func TestRepo_NameCheckSafetyNet(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d)
	ctx := context.Background()
	_, err := r.InsertProject(ctx, base.Project{Name: "A", CreatedAt: time.Now().UTC()})
	if err == nil {
		t.Fatal("CHECK projects_name_len harus menolak")
	}
}

func migratedDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}
