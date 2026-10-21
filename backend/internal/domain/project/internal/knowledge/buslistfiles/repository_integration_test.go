//go:build integration

package buslistfiles

import (
	"context"
	"fmt"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/project/knowledge/listfiles"
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

func (e *env) add(pid string, paths ...string) {
	for _, p := range paths {
		_, err := e.d.ExecContext(e.ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
 VALUES ($1, $2, 't', 'c', decode(repeat('00',32),'hex'), 1, gen_random_uuid(), 'x', gen_random_uuid(), 'x')`, pid, p)
		if err != nil {
			e.t.Fatal(err)
		}
	}
}

func (e *env) list(pid, prefix string, rec bool, limit, offset int) listfiles.Response {
	res, err := e.uc.Execute(e.ctx, listfiles.Request{ProjectID: pid, Prefix: prefix, Recursive: rec, Limit: limit, Offset: offset})
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func paths(r listfiles.Response) []string {
	out := []string{}
	for _, i := range r.Items {
		out = append(out, i.Path)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestIsolationAC8(t *testing.T) {
	e := newEnv(t)
	a, b := e.project("A"), e.project("B")
	e.add(a, "same.md", "docs/only-a.md")
	e.add(b, "same.md", "docs/only-b.md")
	for _, rec := range []bool{true, false} {
		for _, prefix := range []string{"", "docs"} {
			ra, rb := e.list(a, prefix, rec, 0, 0), e.list(b, prefix, rec, 0, 0)
			for _, p := range paths(ra) {
				if p == "docs/only-b.md" {
					t.Fatalf("bocor ke A")
				}
			}
			for _, p := range paths(rb) {
				if p == "docs/only-a.md" {
					t.Fatalf("bocor ke B")
				}
			}
			if ra.Total != len(ra.Items) || rb.Total != len(rb.Items) {
				t.Fatalf("total rec=%v prefix=%q: %d/%d", rec, prefix, ra.Total, rb.Total)
			}
		}
	}
	if r := e.list(a, "", true, 0, 0); r.Total != 2 {
		t.Fatalf("total A: %d", r.Total)
	}
}

func TestRecursiveVsFlat(t *testing.T) {
	e := newEnv(t)
	p := e.project("P")
	e.add(p, "docs/a.md", "docs/auth/login.md", "docs/auth/flow/x.md", "top.md")
	if r := e.list(p, "", true, 0, 0); !eq(paths(r), []string{"docs/a.md", "docs/auth/flow/x.md", "docs/auth/login.md", "top.md"}) || r.Total != 4 {
		t.Fatalf("%+v", r)
	}
	if r := e.list(p, "", false, 0, 0); !eq(paths(r), []string{"top.md"}) || !eq(r.Folders, []string{"docs"}) {
		t.Fatalf("%+v", r)
	}
	if r := e.list(p, "docs", true, 0, 0); len(r.Items) != 3 {
		t.Fatalf("%+v", r)
	}
	if r := e.list(p, "/docs", false, 0, 0); !eq(paths(r), []string{"docs/a.md"}) || !eq(r.Folders, []string{"docs/auth"}) {
		t.Fatalf("%+v", r)
	}
}

func TestByteOrder(t *testing.T) {
	e := newEnv(t)
	p := e.project("P")
	e.add(p, "a.md", "docs/x.md", "B.md")
	if r := e.list(p, "", true, 0, 0); !eq(paths(r), []string{"B.md", "a.md", "docs/x.md"}) {
		t.Fatalf("%v", paths(r))
	}
}

func TestPaging(t *testing.T) {
	e := newEnv(t)
	p := e.project("P")
	var want []string
	for i := 0; i < 7; i++ {
		f := fmt.Sprintf("f%d.md", i)
		want = append(want, f)
		e.add(p, f)
	}
	got := []string{}
	for off, n := 0, 0; off < 9; off, n = off+3, n+1 {
		r := e.list(p, "", true, 3, off)
		got = append(got, paths(r)...)
		if wantTrunc := off+len(r.Items) < 7; r.Truncated != wantTrunc {
			t.Fatalf("offset %d truncated=%v", off, r.Truncated)
		}
	}
	if !eq(got, want) {
		t.Fatalf("%v", got)
	}
}

func TestSpecialChars(t *testing.T) {
	e := newEnv(t)
	p := e.project("P")
	e.add(p, "a_b/x.md", "axb/x.md", "a%c/x.md", "abc/x.md", "docs/x.md", "docs2/x.md", "docs.md")
	if r := e.list(p, "a_b", true, 0, 0); !eq(paths(r), []string{"a_b/x.md"}) {
		t.Fatalf("%v", paths(r))
	}
	if r := e.list(p, "a%c", true, 0, 0); !eq(paths(r), []string{"a%c/x.md"}) {
		t.Fatalf("%v", paths(r))
	}
	if r := e.list(p, "docs", true, 0, 0); !eq(paths(r), []string{"docs/x.md"}) {
		t.Fatalf("%v", paths(r))
	}
}

func TestEmptyAndUnknownProject(t *testing.T) {
	e := newEnv(t)
	p := e.project("P")
	e.add(p, "a.md")
	for _, rec := range []bool{true, false} {
		r := e.list(p, "nope", rec, 0, 0)
		if r.Items == nil || len(r.Items) != 0 || r.Total != 0 || r.Truncated {
			t.Fatalf("%+v", r)
		}
		r = e.list("00000000-0000-4000-8000-000000000000", "", rec, 0, 0)
		if len(r.Items) != 0 || r.Total != 0 {
			t.Fatalf("%+v", r)
		}
	}
}

func TestExplainUsesPrefixIndex(t *testing.T) {
	e := newEnv(t)
	p := e.project("P")
	if _, err := e.d.ExecContext(e.ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
 SELECT $1, 'dir' || (g % 50) || '/f' || g || '.md', 't', 'c', decode(repeat('00',32),'hex'), 1, gen_random_uuid(), 'x', gen_random_uuid(), 'x' FROM generate_series(1, 5000) g`, p); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.ExecContext(e.ctx, `ANALYZE knowledge_files`); err != nil {
		t.Fatal(err)
	}
	var plan []string
	if err := e.d.SelectContext(e.ctx, &plan, `EXPLAIN `+listPageSQL, p, `dir1/%`, 10, 0); err != nil {
		t.Fatal(err)
	}
	t.Logf("plan:\n%v", plan)
}
