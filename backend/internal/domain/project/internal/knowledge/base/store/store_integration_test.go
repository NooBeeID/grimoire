//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/db"
)

var ctx = context.Background()

func setup(t *testing.T) (*db.DB, *Store) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, New(d)
}

func newProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(ctx, &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

var user = uuid.NewString()

func insertFile(t *testing.T, q db.Querier, pid, path, content string) {
	t.Helper()
	f := base.NewFile(pid, path, content, base.Actor{ID: user, Label: "Budi"})
	_, err := q.ExecContext(ctx, `INSERT INTO knowledge_files
		(project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$7,$8)`, pid, path, f.Title, f.Content, f.Hash, f.SizeBytes, user, "Budi")
	if err != nil {
		t.Fatalf("insert %s: %v", path, err)
	}
}

func TestFindByPath_IsolatedPerProject(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	insertFile(t, d, a, "same.md", "# Punya A")
	insertFile(t, d, b, "same.md", "# Punya B")

	f, ok, err := s.FindByPath(ctx, a, "same.md")
	if err != nil || !ok || f.Content != "# Punya A" || f.ProjectID != a || f.Title != "Punya A" ||
		f.CreatedAt.Location().String() != "UTC" || f.CreatedByLabel != "Budi" || len(f.Hash) != 32 || f.SizeBytes != 9 {
		t.Fatalf("%+v %v %v", f, ok, err)
	}
	if _, ok, _ := s.FindByPath(ctx, a, "nope.md"); ok {
		t.Fatal("missing path must not be found")
	}
	if _, ok, err := s.FindByPath(ctx, "bukan-uuid", "same.md"); ok || err != nil {
		t.Fatalf("non-uuid: %v %v", ok, err)
	}
}

func TestCountFiles(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	insertFile(t, d, a, "1.md", "x")
	insertFile(t, d, a, "2.md", "y")
	insertFile(t, d, b, "1.md", "z")
	if n, err := s.CountFiles(ctx, a); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if n, err := s.CountFiles(ctx, "bukan-uuid"); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
}

func TestListMeta_PrefixEscaping(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	for _, p := range []string{"a_b/x.md", "axb/x.md", "a%b/y.md", "docs/1.md", "docs/sub/2.md", "docs2/3.md", "docs.md", `back\slash/z.md`} {
		insertFile(t, d, a, p, "c")
	}
	insertFile(t, d, b, "docs/other.md", "c")

	paths := func(prefix string) []string {
		ms, err := s.ListMeta(ctx, a, base.LikePrefix(prefix))
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, m := range ms {
			out = append(out, m.Path)
		}
		return out
	}
	if got := paths("a_b"); fmt.Sprint(got) != "[a_b/x.md]" {
		t.Fatalf("a_b: %v", got)
	}
	if got := paths("a%b"); fmt.Sprint(got) != "[a%b/y.md]" {
		t.Fatalf("a%%b: %v", got)
	}
	if got := paths(`back\slash`); fmt.Sprint(got) != `[back\slash/z.md]` {
		t.Fatalf("backslash: %v", got)
	}
	if got := paths("docs"); fmt.Sprint(got) != "[docs/1.md docs/sub/2.md]" {
		t.Fatalf("docs: %v", got)
	}
	if got := paths(""); len(got) != 8 {
		t.Fatalf("root: %v", got)
	}
	if ms, err := s.ListMeta(ctx, "bukan-uuid", "%"); err != nil || ms == nil || len(ms) != 0 {
		t.Fatalf("%v %v", ms, err)
	}
}

func TestListPaths(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	insertFile(t, d, a, "b.md", "x")
	insertFile(t, d, a, "a.md", "x")
	insertFile(t, d, b, "c.md", "x")
	got, err := s.ListPaths(ctx, a)
	if err != nil || fmt.Sprint(got) != "[a.md b.md]" {
		t.Fatalf("%v %v", got, err)
	}
	if got, _ := s.ListPaths(ctx, "bukan-uuid"); got == nil || len(got) != 0 {
		t.Fatal("non-uuid must give empty non-nil")
	}
}

func TestSearch_RankTotalPrefixPhrase(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	insertFile(t, d, a, "docs/auth/login.md", "# Login\n\nSingkat.")
	insertFile(t, d, a, "docs/guide.md", "# Panduan\n\n"+strings.Repeat("kata lain ", 200)+" login sekali di sini")
	insertFile(t, d, a, "other/notes.md", "# Catatan\n\nreset password lewat email")
	insertFile(t, d, b, "login.md", "# Login B uniqueB")

	hits, total, err := s.Search(ctx, a, `"login"`, "%", 10)
	if err != nil || total != 2 || len(hits) != 2 {
		t.Fatalf("%v total=%d err=%v", hits, total, err)
	}
	if hits[0].Path != "docs/auth/login.md" || hits[0].Score <= hits[1].Score {
		t.Fatalf("rank order: %+v", hits)
	}
	if !strings.Contains(hits[0].Headline, base.HighlightStart) {
		t.Fatalf("headline: %q", hits[0].Headline)
	}

	hits, total, _ = s.Search(ctx, a, `"login"`, base.LikePrefix("docs/auth"), 10)
	if total != 1 || len(hits) != 1 || hits[0].Path != "docs/auth/login.md" {
		t.Fatalf("prefix: %v %d", hits, total)
	}
	hits, total, _ = s.Search(ctx, a, `"reset password"`, "%", 10)
	if total != 1 || hits[0].Path != "other/notes.md" {
		t.Fatalf("phrase: %v %d", hits, total)
	}
	hits, total, _ = s.Search(ctx, a, `"login"`, "%", 1)
	if total != 2 || len(hits) != 1 {
		t.Fatalf("limit keeps total: %v %d", hits, total)
	}
	hits, total, err = s.Search(ctx, a, `"tidakada"`, "%", 10)
	if err != nil || total != 0 || hits == nil || len(hits) != 0 {
		t.Fatalf("no hit: %v %d %v", hits, total, err)
	}
	if hits, total, _ := s.Search(ctx, a, `"uniqueb"`, "%", 10); total != 0 || len(hits) != 0 {
		t.Fatalf("isolation: %v", hits)
	}
	if hits, total, err := s.Search(ctx, "bukan-uuid", `"login"`, "%", 10); err != nil || total != 0 || len(hits) != 0 {
		t.Fatalf("non-uuid: %v %v", hits, err)
	}
}

func TestSearch_PathAndTitleMatch(t *testing.T) {
	d, s := setup(t)
	a := newProject(t, d, "Alpha")
	insertFile(t, d, a, "docs/deploy.md", "tidak ada kata itu")
	hits, total, err := s.Search(ctx, a, `"deploy"`, "%", 5)
	if err != nil || total != 1 || hits[0].Path != "docs/deploy.md" {
		t.Fatalf("path match: %v %d %v", hits, total, err)
	}
}

func TestSearch_ContentWithMarkerBytes(t *testing.T) {
	d, s := setup(t)
	a := newProject(t, d, "Alpha")
	insertFile(t, d, a, "m.md", "awal \x01 palsu \x02 login akhir")
	hits, _, err := s.Search(ctx, a, `"login"`, "%", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("%v %v", hits, err)
	}
	segs := base.ParseHeadline(hits[0].Headline)
	n := 0
	for _, sg := range segs {
		if sg.Highlight {
			n++
			if sg.Text != "login" {
				t.Fatalf("highlighted %q", sg.Text)
			}
		}
	}
	if n != 1 {
		t.Fatalf("segments: %+v", segs)
	}
}

func TestLargeContentWithUniqueWords(t *testing.T) {
	d, s := setup(t)
	a := newProject(t, d, "Alpha")
	var sb strings.Builder
	for sb.Len() < 1<<20-16 {
		fmt.Fprintf(&sb, "w%d ", sb.Len())
	}
	content := sb.String()
	insertFile(t, d, a, "big.md", content)
	if n, _ := s.CountFiles(ctx, a); n != 1 {
		t.Fatal("big file must insert")
	}
}

func TestInTxCommitRollbackAndLock(t *testing.T) {
	d, s := setup(t)
	a := newProject(t, d, "Alpha")

	boom := errors.New("boom")
	err := s.InTx(ctx, func(tx *Store) error {
		insertFile(t, tx.Q(), a, "rb.md", "x")
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if n, _ := s.CountFiles(ctx, a); n != 0 {
		t.Fatal("rollback expected")
	}
	if err := s.InTx(ctx, func(tx *Store) error {
		insertFile(t, tx.Q(), a, "ok.md", "x")
		return tx.InTx(ctx, func(inner *Store) error { return nil }) // nested: pakai tx yang sama
	}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountFiles(ctx, a); n != 1 {
		t.Fatal("commit expected")
	}

	// LockProject memblokir tx kedua sampai tx pertama selesai.
	var order atomic.Int32
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		done <- s.InTx(ctx, func(tx *Store) error {
			if err := tx.LockProject(ctx, a); err != nil {
				return err
			}
			close(locked)
			<-release
			order.Store(1)
			return nil
		})
	}()
	<-locked
	go func() {
		done <- s.InTx(ctx, func(tx *Store) error {
			if err := tx.LockProject(ctx, a); err != nil {
				return err
			}
			if order.Load() != 1 {
				return errors.New("second tx acquired lock before first finished")
			}
			return nil
		})
	}()
	time.Sleep(200 * time.Millisecond)
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.LockProject(ctx, "bukan-uuid"); err != nil {
		t.Fatalf("non-uuid lock must be no-op: %v", err)
	}
}

func TestIsUniqueViolation(t *testing.T) {
	d, s := setup(t)
	a := newProject(t, d, "Alpha")
	insertFile(t, d, a, "dup.md", "x")
	f := base.NewFile(a, "dup.md", "y", base.Actor{ID: user, Label: "B"})
	_, err := d.ExecContext(ctx, `INSERT INTO knowledge_files
		(project_id, path, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
		VALUES ($1,$2,$3,$4,$5,$6,'B',$6,'B')`, a, f.Path, f.Content, f.Hash, f.SizeBytes, user)
	if !IsUniqueViolation(err) {
		t.Fatalf("err=%v", err)
	}
	if IsUniqueViolation(errors.New("x")) || IsUniqueViolation(nil) {
		t.Fatal("false positives")
	}
	_ = s
}

func TestCascadeOnProjectDelete(t *testing.T) {
	d, s := setup(t)
	a := newProject(t, d, "Alpha")
	insertFile(t, d, a, "x.md", "x")
	if _, err := d.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountFiles(ctx, a); n != 0 {
		t.Fatal("cascade expected")
	}
}

func TestGetFile_NoRows(t *testing.T) {
	_, s := setup(t)
	if _, ok, err := s.GetFile(ctx, `SELECT `+FileColumns+` FROM knowledge_files WHERE id = $1`, uuid.NewString()); ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	_ = sqlx.Named
}
