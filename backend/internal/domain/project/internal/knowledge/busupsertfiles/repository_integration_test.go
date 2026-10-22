//go:build integration

package busupsertfiles

import (
	"context"
	"fmt"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
)

type env struct {
	t   *testing.T
	d   *db.DB
	uc  *Usecase
	a   *fakeAudit
	ctx context.Context
}

func newEnv(t *testing.T, l base.Limits) *env {
	ctx := context.Background()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	a := &fakeAudit{log: &[]string{}}
	return &env{t: t, d: d, ctx: ctx, a: a, uc: NewUsecase(NewRepo(store.New(d)), a, l)}
}

func (e *env) project(name string) string {
	var id string
	if err := e.d.GetContext(e.ctx, &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) seed(pid, path, content string) {
	f := base.NewFile(pid, path, content, base.Actor{ID: uid, Label: "seed"})
	if err := store.New(e.d).InsertFiles(e.ctx, []base.File{f}); err != nil {
		e.t.Fatal(err)
	}
}

type row struct {
	Path      string    `db:"path"`
	Content   string    `db:"content"`
	CreatedBy string    `db:"created_by"`
	Label     string    `db:"created_by_label"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (e *env) rows(pid string) map[string]row {
	var rs []row
	if err := e.d.SelectContext(e.ctx, &rs, `SELECT path, content, created_by::text, created_by_label, updated_at FROM knowledge_files WHERE project_id = $1`, pid); err != nil {
		e.t.Fatal(err)
	}
	m := map[string]row{}
	for _, r := range rs {
		m[r.Path] = r
	}
	return m
}

func (e *env) run(pid, prefix string, items ...upsertfiles.Item) upsertfiles.Response {
	rq := req(prefix, items...)
	rq.ProjectID = pid
	res, err := e.uc.Execute(e.ctx, rq)
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func TestPrefixScopeAndNoDelete(t *testing.T) {
	e := newEnv(t, lim)
	pid := e.project("A")
	e.seed(pid, "other/x.md", "x")
	e.seed(pid, "README.md", "r")
	e.seed(pid, "guides/old.md", "old")
	e.seed(pid, "guides/keep.md", "keep")
	before := e.rows(pid)

	res := e.run(pid, "guides", item(0, "new.md", "n"), item(1, "old.md", "changed"))
	if res.Summary.Created != 1 || res.Summary.Updated != 1 {
		t.Fatalf("%+v", res)
	}
	after := e.rows(pid)
	if len(after) != 5 {
		t.Fatalf("jumlah file %d", len(after))
	}
	for _, p := range []string{"other/x.md", "README.md", "guides/keep.md"} {
		if after[p] != before[p] {
			t.Fatalf("%s berubah", p)
		}
	}
	if after["guides/old.md"].Content != "changed" {
		t.Fatal("old.md tidak diperbarui")
	}
	if n := after["guides/new.md"]; n.CreatedBy != uid || n.Label != "Rina" {
		t.Fatalf("%+v", n)
	}

	again := e.run(pid, "guides", item(0, "new.md", "n"), item(1, "old.md", "changed"))
	if again.Summary.Unchanged != 2 {
		t.Fatalf("%+v", again)
	}
	if e.rows(pid)["guides/old.md"].UpdatedAt != after["guides/old.md"].UpdatedAt {
		t.Fatal("updated_at berubah pada unchanged")
	}
}

func TestProjectIsolation(t *testing.T) {
	e := newEnv(t, lim)
	a, b := e.project("A"), e.project("B")
	e.seed(b, "docs/a.md", "B-content")
	res := e.run(a, "", item(0, "docs/a.md", "A-content"))
	if res.Summary.Created != 1 {
		t.Fatalf("%+v", res)
	}
	if e.rows(b)["docs/a.md"].Content != "B-content" {
		t.Fatal("project B berubah")
	}
}

func TestLargeBatchChunks(t *testing.T) {
	e := newEnv(t, lim)
	pid := e.project("A")
	var items []upsertfiles.Item
	for i := 0; i < 250; i++ {
		items = append(items, item(i, fmt.Sprintf("d/f%03d.md", i), "c"))
	}
	res := e.run(pid, "", items...)
	if res.Summary.Created != 250 || len(e.rows(pid)) != 250 {
		t.Fatalf("%+v", res.Summary)
	}
}

func TestQuotaPerProject(t *testing.T) {
	l := lim
	l.MaxProjectFiles = 2
	e := newEnv(t, l)
	pid := e.project("A")
	e.seed(pid, "a.md", "a")
	res := e.run(pid, "", item(0, "b.md", "b"), item(1, "c.md", "c"))
	if res.Summary.Created != 1 || res.Summary.Rejected != 1 || len(e.rows(pid)) != 2 {
		t.Fatalf("%+v", res)
	}
}
