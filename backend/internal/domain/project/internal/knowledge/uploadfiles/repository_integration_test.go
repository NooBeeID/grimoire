//go:build integration

package uploadfiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
)

var ictx = context.Background()
var iuser = uuid.NewString()
var iactor = base.Actor{ID: iuser, Label: "Budi"}

func isetup(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(ictx, d, nil); err != nil {
		t.Fatal(err)
	}
	return d, NewRepo(store.New(d))
}

func iproject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(ictx, &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInsertFilesChunksAndColumns(t *testing.T) {
	d, r := isetup(t)
	pid := iproject(t, d, "A")
	var fs []base.File
	for i := 0; i < 250; i++ {
		fs = append(fs, base.NewFile(pid, fmt.Sprintf("d/f%03d.md", i), fmt.Sprintf("# T%d\r\nkata é\n\nx", i), iactor))
	}
	if err := r.InsertFiles(ictx, fs); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.GetContext(ictx, &n, `SELECT count(*) FROM knowledge_files WHERE project_id=$1`, pid)
	if n != 250 {
		t.Fatalf("count %d", n)
	}
	got, ok, err := store.New(d).FindByPath(ictx, pid, "d/f007.md")
	if err != nil || !ok || got.Content != fs[7].Content || !bytes.Equal(got.Hash, fs[7].Hash) ||
		got.Title != "T7" || got.CreatedBy != iuser || got.UpdatedByLabel != "Budi" {
		t.Fatalf("%+v %v %v", got, ok, err)
	}
	var hits int
	_ = d.GetContext(ictx, &hits, `SELECT count(*) FROM knowledge_files WHERE project_id=$1 AND search @@ to_tsquery('simple','kata')`, pid)
	if hits != 250 {
		t.Fatalf("search %d", hits)
	}
}

func TestUpdateFilesAndIsolation(t *testing.T) {
	d, r := isetup(t)
	a, b := iproject(t, d, "A"), iproject(t, d, "B")
	for _, p := range []string{a, b} {
		if err := r.InsertFiles(ictx, []base.File{base.NewFile(p, "x.md", "# Lama", iactor)}); err != nil {
			t.Fatal(err)
		}
	}
	other := base.Actor{ID: uuid.NewString(), Label: "Siti"}
	if err := r.UpdateFiles(ictx, []base.File{base.NewFile(a, "x.md", "# Baru", other)}); err != nil {
		t.Fatal(err)
	}
	s := store.New(d)
	fa, _, _ := s.FindByPath(ictx, a, "x.md")
	fb, _, _ := s.FindByPath(ictx, b, "x.md")
	if fa.Title != "Baru" || fa.UpdatedByLabel != "Siti" || fa.CreatedByLabel != "Budi" || !fa.UpdatedAt.After(fa.CreatedAt) {
		t.Fatalf("%+v", fa)
	}
	if fb.Content != "# Lama" {
		t.Fatalf("project B tersentuh: %+v", fb)
	}
	if err := r.UpdateFiles(ictx, []base.File{base.NewFile(a, "tidak-ada.md", "x", iactor)}); err == nil {
		t.Fatal("want rows affected error")
	}
}

func TestExistingHashes(t *testing.T) {
	d, r := isetup(t)
	a, b := iproject(t, d, "A"), iproject(t, d, "B")
	fa := base.NewFile(a, "x.md", "A", iactor)
	_ = r.InsertFiles(ictx, []base.File{fa})
	_ = r.InsertFiles(ictx, []base.File{base.NewFile(b, "x.md", "B", iactor)})
	m, err := r.ExistingHashes(ictx, a, []string{"x.md", "none.md"})
	if err != nil || len(m) != 1 || !bytes.Equal(m["x.md"], fa.Hash) {
		t.Fatalf("%v %v", m, err)
	}
	if m, err := r.ExistingHashes(ictx, a, nil); err != nil || len(m) != 0 {
		t.Fatalf("%v %v", m, err)
	}
}

func TestRollbackAtomic(t *testing.T) {
	d, r := isetup(t)
	pid := iproject(t, d, "A")
	err := r.InTx(ictx, func(tx repository) error {
		if err := tx.InsertFiles(ictx, []base.File{base.NewFile(pid, "a.md", "a", iactor)}); err != nil {
			return err
		}
		return tx.UpdateFiles(ictx, []base.File{base.NewFile(pid, "ghost.md", "g", iactor)})
	})
	if err == nil {
		t.Fatal("want error")
	}
	var n int
	_ = d.GetContext(ictx, &n, `SELECT count(*) FROM knowledge_files WHERE project_id=$1`, pid)
	if n != 0 {
		t.Fatalf("rollback gagal: %d", n)
	}
	_ = errors.New
}
