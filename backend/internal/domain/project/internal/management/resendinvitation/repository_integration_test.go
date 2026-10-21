//go:build integration

package resendinvitation

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

func seedProject(t *testing.T, d *db.DB, name, owner, member string) string {
	t.Helper()
	ctx := context.Background()
	var pid string
	if err := d.GetContext(ctx, &pid, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,'owner'),($1,$3,'member')`, pid, owner, member); err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestRepo_FindAccessAndListMembers(t *testing.T) {
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := newRepo(store.New(d))
	ctx := context.Background()
	owner, member := uuid.NewString(), uuid.NewString()
	pid := seedProject(t, d, "Alpha", owner, member)
	otherPID := seedProject(t, d, "Beta", uuid.NewString(), uuid.NewString())

	p, role, ok, err := r.FindAccess(ctx, pid, owner)
	if err != nil || !ok || role != "owner" || p.Name != "Alpha" {
		t.Fatalf("%+v %q %v %v", p, role, ok, err)
	}
	if _, role, ok, _ := r.FindAccess(ctx, pid, member); !ok || role != "member" {
		t.Fatalf("member: %q %v", role, ok)
	}
	if _, role, ok, _ := r.FindAccess(ctx, pid, uuid.NewString()); !ok || role != "" {
		t.Fatalf("non-anggota: %q %v", role, ok)
	}

	ms, err := r.ListMembers(ctx, pid)
	if err != nil || len(ms) != 2 {
		t.Fatalf("%+v %v", ms, err)
	}
	for _, m := range ms {
		if m.ProjectID != pid {
			t.Fatalf("project lain bocor: %+v (lain=%s)", m, otherPID)
		}
	}
}
