//go:build integration

package countfiles

import (
	"context"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
)

func TestRepositoryCountByProject(t *testing.T) {
	ctx := context.Background()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	newProject := func(name string) string {
		var id string
		if err := d.GetContext(ctx, &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	addFiles := func(pid string, n int) {
		for i := 0; i < n; i++ {
			_, err := d.ExecContext(ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
 VALUES ($1, $2, 't', 'c', decode(repeat('00',32),'hex'), 1, gen_random_uuid(), 'x', gen_random_uuid(), 'x')`, pid, "f"+string(rune('a'+i))+".md")
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	a, b, c, other := newProject("A"), newProject("B"), newProject("C"), newProject("D")
	addFiles(a, 3)
	addFiles(b, 1)
	addFiles(other, 2)

	uc := NewUsecase(NewRepository(store.New(d)))
	got, err := uc.Count(ctx, []string{a, b, c, "bukan-uuid"})
	if err != nil {
		t.Fatal(err)
	}
	if got[a] != 3 || got[b] != 1 || got[c] != 0 || got["bukan-uuid"] != 0 || len(got) != 4 {
		t.Fatalf("got %v", got)
	}
	if _, ok := got[other]; ok {
		t.Fatal("project tak diminta bocor")
	}

	if _, err := d.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, a); err != nil {
		t.Fatal(err)
	}
	got, _ = uc.Count(ctx, []string{a})
	if got[a] != 0 {
		t.Fatalf("cascade: %v", got)
	}
}
