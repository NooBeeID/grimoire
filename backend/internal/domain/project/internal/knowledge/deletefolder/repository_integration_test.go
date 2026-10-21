//go:build integration

package deletefolder

import (
	"context"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
)

func TestDeleteByPrefix(t *testing.T) {
	ctx := context.Background()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatal(err)
	}
	mk := func(name string) string {
		var id string
		if err := d.GetContext(ctx, &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := mk("A"), mk("B")
	ins := func(pid, path string) {
		if _, err := d.ExecContext(ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
 VALUES ($1,$2,$2,'x',sha256('x'::bytea),1,gen_random_uuid(),'x',gen_random_uuid(),'x')`, pid, path); err != nil {
			t.Fatal(err)
		}
	}
	count := func(pid string) int {
		var n int
		if err := d.GetContext(ctx, &n, `SELECT count(*) FROM knowledge_files WHERE project_id=$1`, pid); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, p := range []string{"docs/a.md", "docs/auth/login.md", "docs/auth/flow/x.md", "docs/auth/flow/deep/er/y.md",
		"docs2/x.md", "docs.md", "docs-old/y.md", "readme.md", "Docs/x.md",
		"a_b/x.md", "axb/y.md", "100%/z.md", "100x/w.md", `back\slash/q.md`, "back/slash/q.md"} {
		ins(a, p)
	}
	ins(b, "docs/a.md")
	ins(b, "docs/b.md")
	r := newRepo(store.New(d))
	del := func(prefix string) int {
		n, err := r.DeleteByPrefix(ctx, a, base.LikePrefix(prefix))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	if n := del("docs/auth/flow/deep"); n != 1 {
		t.Fatalf("deep: %d", n)
	}
	if n := del("docs"); n != 3 {
		t.Fatalf("docs: %d", n)
	}
	if n := del("docs"); n != 0 {
		t.Fatalf("docs again: %d", n)
	}
	if count(b) != 2 {
		t.Fatal("project B terpengaruh")
	}
	if n := del("a_b"); n != 1 {
		t.Fatalf("a_b: %d", n)
	}
	if n := del("100%"); n != 1 {
		t.Fatalf("100%%: %d", n)
	}
	if n := del(`back\slash`); n != 1 {
		t.Fatalf("backslash: %d", n)
	}
	// sisa: docs2/x.md docs.md docs-old/y.md readme.md Docs/x.md axb/y.md 100x/w.md back/slash/q.md
	if got := count(a); got != 8 {
		t.Fatalf("sisa: %d", got)
	}
	if n, err := r.DeleteByPrefix(ctx, "not-a-uuid", "docs/%"); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
}
