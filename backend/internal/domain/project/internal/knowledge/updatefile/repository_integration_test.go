//go:build integration

package updatefile

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
)

var ctx = context.Background()

func setupDB(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatal(err)
	}
	return d, NewRepo(store.New(d))
}

func newProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(ctx, &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func insert(t *testing.T, d *db.DB, pid, path, content string, a base.Actor) {
	t.Helper()
	f := base.NewFile(pid, path, content, a)
	if _, err := d.ExecContext(ctx, `INSERT INTO knowledge_files
		(project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$7,$8)`, pid, path, f.Title, f.Content, f.Hash, f.SizeBytes, a.ID, a.Label); err != nil {
		t.Fatal(err)
	}
}

var (
	creator = base.Actor{ID: uuid.NewString(), Label: "Budi"}
	editor  = base.Actor{ID: uuid.NewString(), Label: "Sari"}
)

func next(pid, path, content string) base.File {
	return base.File{ProjectID: pid, Path: path}.WithContent(content, editor)
}

func TestUpdateIfVersion_Success(t *testing.T) {
	d, r := setupDB(t)
	p := newProject(t, d, "A")
	insert(t, d, p, "a.md", "# Lama", creator)
	before, _, _ := r.FindByPath(ctx, p, "a.md")
	time.Sleep(5 * time.Millisecond)

	got, bb, found, err := r.UpdateIfVersion(ctx, p, "a.md", before.Hash, next(p, "a.md", "# Baru\nisi"))
	if err != nil || !found {
		t.Fatal(found, err)
	}
	if bb != len("# Lama") || got.Content != "# Baru\nisi" || got.Title != "Baru" || got.SizeBytes != 10 ||
		string(got.Hash) != string(base.Hash("# Baru\nisi")) || !got.UpdatedAt.After(before.UpdatedAt) ||
		got.UpdatedBy != editor.ID || got.UpdatedByLabel != "Sari" ||
		got.ID != before.ID || !got.CreatedAt.Equal(before.CreatedAt) || got.CreatedBy != creator.ID || got.CreatedByLabel != "Budi" {
		t.Fatalf("%+v", got)
	}
	stored, _, _ := r.FindByPath(ctx, p, "a.md")
	if stored.Content != "# Baru\nisi" {
		t.Fatal("tidak tersimpan")
	}
}

func TestUpdateIfVersion_NoMatch(t *testing.T) {
	d, r := setupDB(t)
	a, b := newProject(t, d, "A"), newProject(t, d, "B")
	insert(t, d, a, "a.md", "asli", creator)
	insert(t, d, b, "a.md", "milik B", creator)
	orig, _, _ := r.FindByPath(ctx, a, "a.md")

	if _, _, found, err := r.UpdateIfVersion(ctx, a, "a.md", base.Hash("lain"), next(a, "a.md", "x")); found || err != nil {
		t.Fatal(found, err)
	}
	if s, _, _ := r.FindByPath(ctx, a, "a.md"); s.Content != "asli" {
		t.Fatal("berubah")
	}
	if _, _, found, _ := r.UpdateIfVersion(ctx, a, "tidak-ada.md", orig.Hash, next(a, "tidak-ada.md", "x")); found {
		t.Fatal("path tidak ada")
	}
	// isolasi: hash project A tidak boleh menyentuh baris project B
	insert(t, d, a, "z.md", "milik B", creator)
	if _, _, found, _ := r.UpdateIfVersion(ctx, b, "a.md", base.Hash("milik B"), next(b, "a.md", "B baru")); !found {
		t.Fatal("B harus bisa")
	}
	if s, _, _ := r.FindByPath(ctx, a, "z.md"); s.Content != "milik B" {
		t.Fatal("A tersentuh")
	}
	if _, _, found, err := r.UpdateIfVersion(ctx, "bukan-uuid", "a.md", orig.Hash, next("bukan-uuid", "a.md", "x")); found || err != nil {
		t.Fatal(found, err)
	}
}

func TestUpdateIfVersion_Concurrent(t *testing.T) {
	d, r := setupDB(t)
	p := newProject(t, d, "A")
	for i := 0; i < 50; i++ {
		path := "c" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".md"
		insert(t, d, p, path, "v0", creator)
		exp := base.Hash("v0")
		var wg sync.WaitGroup
		start := make(chan struct{})
		var mu sync.Mutex
		wins := map[string]bool{}
		for _, c := range []string{"isi A", "isi B"} {
			wg.Add(1)
			go func(c string) {
				defer wg.Done()
				<-start
				_, _, found, err := r.UpdateIfVersion(ctx, p, path, exp, next(p, path, c))
				if err != nil {
					t.Error(err)
				}
				mu.Lock()
				wins[c] = found
				mu.Unlock()
			}(c)
		}
		close(start)
		wg.Wait()
		if wins["isi A"] == wins["isi B"] {
			t.Fatalf("iterasi %d: tepat satu pemenang: %v", i, wins)
		}
		winner := "isi A"
		if wins["isi B"] {
			winner = "isi B"
		}
		if s, _, _ := r.FindByPath(ctx, p, path); s.Content != winner {
			t.Fatalf("isi %q != pemenang %q", s.Content, winner)
		}
	}
}

func TestUpdateIfVersion_SearchEmptyAndLarge(t *testing.T) {
	d, r := setupDB(t)
	p := newProject(t, d, "A")
	insert(t, d, p, "a.md", "kataunikalama", creator)
	_, _, found, err := r.UpdateIfVersion(ctx, p, "a.md", base.Hash("kataunikalama"), next(p, "a.md", "kataunikbaru"))
	if !found || err != nil {
		t.Fatal(found, err)
	}
	count := func(q string) int {
		var n int
		if err := d.GetContext(ctx, &n, `SELECT count(*) FROM knowledge_files WHERE project_id=$1 AND search @@ websearch_to_tsquery('simple',$2)`, p, q); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("kataunikalama") != 0 || count("kataunikbaru") != 1 {
		t.Fatal("kolom search tidak ikut berubah")
	}

	var sb strings.Builder
	for sb.Len() < 1<<20-16 {
		sb.WriteString("w" + string(rune('a'+sb.Len()%26)) + " ")
	}
	big := sb.String()
	if _, _, found, err := r.UpdateIfVersion(ctx, p, "a.md", base.Hash("kataunikbaru"), next(p, "a.md", big)); !found || err != nil {
		t.Fatal(found, err)
	}
	g, bb, found, err := r.UpdateIfVersion(ctx, p, "a.md", base.Hash(big), next(p, "a.md", ""))
	if !found || err != nil || g.SizeBytes != 0 || bb != len(big) {
		t.Fatalf("%v %v %+v", found, err, g)
	}
}
