//go:build integration

package buslistchanges

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

type env struct {
	t   *testing.T
	d   *db.DB
	uc  *Usecase
	ctx context.Context
}

func newEnv(t *testing.T) *env {
	ctx := context.Background()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &env{t: t, d: d, ctx: ctx, uc: New(NewRepo(store.New(d)))}
}

func (e *env) project(name string) string {
	var id string
	if err := e.d.GetContext(e.ctx, &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// add menyisipkan file dengan created_at = updated_at = ts.
func (e *env) add(pid, path string, ts time.Time) {
	_, err := e.d.ExecContext(e.ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label, created_at, updated_at)
 VALUES ($1, $2, 't', 'c', decode(repeat('00',32),'hex'), 1, gen_random_uuid(), 'x', gen_random_uuid(), 'x', $3, $3)`, pid, path, ts)
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) list(pid string, since time.Time, prefix string, limit int) listchanges.Response {
	res, err := e.uc.Execute(e.ctx, listchanges.Request{ProjectID: pid, Since: since, Prefix: prefix, Limit: limit})
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func paths(r listchanges.Response) []string {
	out := []string{}
	for _, i := range r.Items {
		out = append(out, i.Path)
	}
	return out
}

func same(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

func TestRepo_SinceFilterAndOrder(t *testing.T) {
	e := newEnv(t)
	p := e.project("since")
	now := time.Now().UTC()
	e.add(p, "old.md", now.Add(-72*time.Hour))
	e.add(p, "b.md", now.Add(-2*time.Hour))
	e.add(p, "c.md", now.Add(-1*time.Hour))
	res := e.list(p, now.Add(-24*time.Hour), "", 10)
	if !same(paths(res), []string{"c.md", "b.md"}) || res.Total != 2 {
		t.Fatalf("%v total=%d", paths(res), res.Total)
	}
}

func TestRepo_TieBreakByPath(t *testing.T) {
	e := newEnv(t)
	p := e.project("tie")
	ts := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	e.add(p, "a.md", ts)
	e.add(p, "B.md", ts)
	res := e.list(p, ts.Add(-time.Hour), "", 10)
	if !same(paths(res), []string{"B.md", "a.md"}) {
		t.Fatalf("%v", paths(res))
	}
}

func TestRepo_CreatedFlagSource(t *testing.T) {
	e := newEnv(t)
	p := e.project("created")
	if _, err := e.d.ExecContext(e.ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
 VALUES ($1, 'n.md', 't', 'c', decode(repeat('00',32),'hex'), 1, gen_random_uuid(), 'x', gen_random_uuid(), 'x')`, p); err != nil {
		t.Fatal(err)
	}
	since := time.Now().UTC().Add(-time.Hour)
	it := e.list(p, since, "", 10).Items[0]
	if !it.CreatedAt.Equal(it.UpdatedAt) {
		t.Fatalf("baru: %v != %v", it.CreatedAt, it.UpdatedAt)
	}
	if _, err := e.d.ExecContext(e.ctx, `UPDATE knowledge_files SET updated_at = now() + interval '1 second' WHERE project_id=$1`, p); err != nil {
		t.Fatal(err)
	}
	it = e.list(p, since, "", 10).Items[0]
	if it.CreatedAt.Equal(it.UpdatedAt) {
		t.Fatal("setelah update harus beda")
	}
}

func TestRepo_IsolationAndPrefix(t *testing.T) {
	e := newEnv(t)
	a, b := e.project("A"), e.project("B")
	now := time.Now().UTC().Add(-time.Minute)
	e.add(a, "docs/a.md", now)
	e.add(a, "docsx/b.md", now)
	e.add(a, "x_y/c.md", now)
	e.add(a, "xzy/d.md", now)
	e.add(b, "docs/secret.md", now)
	since := now.Add(-time.Hour)
	if got := paths(e.list(a, since, "docs", 10)); !same(got, []string{"docs/a.md"}) {
		t.Fatalf("docs: %v", got)
	}
	if got := paths(e.list(a, since, "x_y", 10)); !same(got, []string{"x_y/c.md"}) {
		t.Fatalf("_ literal: %v", got)
	}
	for _, p := range paths(e.list(a, since, "", 10)) {
		if strings.Contains(p, "secret") {
			t.Fatal("bocor antar project")
		}
	}
}

func TestRepo_TotalBeyondLimit(t *testing.T) {
	e := newEnv(t)
	p := e.project("total")
	now := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < 5; i++ {
		e.add(p, fmt.Sprintf("f%d.md", i), now.Add(time.Duration(i)*time.Second))
	}
	res := e.list(p, now.Add(-time.Hour), "", 2)
	if len(res.Items) != 2 || res.Total != 5 {
		t.Fatalf("len=%d total=%d", len(res.Items), res.Total)
	}
}

func TestExplainUsesUpdatedIndex(t *testing.T) {
	e := newEnv(t)
	p := e.project("explain")
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	if _, err := e.d.ExecContext(e.ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label, created_at, updated_at)
 SELECT $1, 'bulk/' || g || '.md', 't', 'c', decode(repeat('00',32),'hex'), 1, gen_random_uuid(), 'x', gen_random_uuid(), 'x', $2, $2
 FROM generate_series(1, 5000) g`, p, old); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		e.add(p, fmt.Sprintf("new/%d.md", i), time.Now().UTC())
	}
	if _, err := e.d.ExecContext(e.ctx, `ANALYZE knowledge_files`); err != nil {
		t.Fatal(err)
	}
	var lines []string
	if err := e.d.SelectContext(e.ctx, &lines, "EXPLAIN "+listChangesSQL, p, time.Now().UTC().Add(-time.Hour), "%", 20); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(lines, "\n")
	t.Log("\n" + plan)
	if !strings.Contains(plan, "knowledge_files_project_updated_idx") {
		t.Fatalf("indeks tidak dipakai:\n%s", plan)
	}
}
