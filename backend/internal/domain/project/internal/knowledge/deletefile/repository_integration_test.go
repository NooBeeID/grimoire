//go:build integration

package deletefile

import (
	"context"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
)

func TestDeleteByPath(t *testing.T) {
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
	ins := func(pid, path, content string) {
		if _, err := d.ExecContext(ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
 VALUES ($1,$2,$2,$3,sha256(convert_to($3,'UTF8')),length($3),gen_random_uuid(),'x',gen_random_uuid(),'x')`,
			pid, path, content); err != nil {
			t.Fatal(err)
		}
	}
	count := func(pid, path string) int {
		var n int
		if err := d.GetContext(ctx, &n, `SELECT count(*) FROM knowledge_files WHERE project_id=$1 AND path=$2`, pid, path); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, p := range []string{"same.md", "docs/a.md", "docs/b.md", "docs/a.md.txt", "Docs/A.md", "a_b.md", "axb.md"} {
		ins(a, p, "12345")
	}
	ins(b, "same.md", "abc")
	r := newRepo(store.New(d))

	if n, ok, err := r.DeleteByPath(ctx, a, "same.md"); err != nil || !ok || n != 5 {
		t.Fatalf("%d %v %v", n, ok, err)
	}
	if count(a, "same.md") != 0 || count(b, "same.md") != 1 {
		t.Fatal("isolasi project gagal")
	}
	if _, ok, err := r.DeleteByPath(ctx, a, "same.md"); err != nil || ok {
		t.Fatalf("hapus kedua: %v %v", ok, err)
	}
	if _, ok, err := r.DeleteByPath(ctx, a, "nope.md"); err != nil || ok {
		t.Fatal("path tidak ada")
	}
	if _, ok, err := r.DeleteByPath(ctx, "bukan-uuid", "a.md"); err != nil || ok {
		t.Fatal("uuid invalid")
	}
	if _, ok, _ := r.DeleteByPath(ctx, a, "Docs/A.md"); !ok || count(a, "docs/a.md") != 1 {
		t.Fatal("case sensitivity")
	}
	if _, ok, _ := r.DeleteByPath(ctx, a, "docs/a.md"); !ok ||
		count(a, "docs/b.md") != 1 || count(a, "docs/a.md.txt") != 1 {
		t.Fatal("file tetangga ikut terhapus")
	}
	if _, ok, _ := r.DeleteByPath(ctx, a, "a_b.md"); !ok || count(a, "axb.md") != 1 {
		t.Fatal("underscore harus literal")
	}
}
