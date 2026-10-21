//go:build integration

package busreadfile

import (
	"context"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/project/knowledge/readfile"
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
	return &env{t: t, d: d, ctx: ctx, uc: NewUsecase(store.New(d), base.Limits{MaxReadBytes: 1 << 20})}
}

func (e *env) project(name string) string {
	var id string
	if err := e.d.GetContext(e.ctx, &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) add(pid, path, content string) {
	_, err := e.d.ExecContext(e.ctx, `INSERT INTO knowledge_files
 (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
 VALUES ($1, $2, 't', $3, decode(repeat('00',32),'hex'), $4, gen_random_uuid(), 'x', gen_random_uuid(), 'x')`, pid, path, content, len(content))
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) read(pid, path string, off, limit int) readfile.Response {
	res, err := e.uc.Execute(e.ctx, readfile.Request{ProjectID: pid, Path: path, Offset: off, Limit: limit})
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func TestIntegrationMultibyteChaining(t *testing.T) {
	e := newEnv(t)
	p := e.project("p")
	content := strings.Repeat("café 😀 世界\n", 300)
	e.add(p, "m.md", content)
	var sb strings.Builder
	off := 0
	for i := 0; i < len(content)+2; i++ {
		r := e.read(p, "m.md", off, 7)
		if r.TotalBytes != len(content) {
			t.Fatalf("total %d", r.TotalBytes)
		}
		sb.WriteString(r.Content)
		if !r.Truncated {
			break
		}
		off = r.NextOffset
	}
	if sb.String() != content {
		t.Fatal("isi berbeda")
	}
}

func TestIntegrationIsolationAndCase(t *testing.T) {
	e := newEnv(t)
	a, b := e.project("a"), e.project("b")
	e.add(a, "docs/a.md", "A")
	e.add(b, "same.md", "B")
	e.add(b, "docs/login.md", "B2")

	if r := e.read(a, "same.md", 0, 0); r.Found {
		t.Fatal("bocor")
	}
	r := e.read(a, "docs/logn.md", 0, 0)
	for _, s := range r.Similar {
		if s == "docs/login.md" {
			t.Fatal("similar bocor")
		}
	}
	if r := e.read(a, "Docs/A.md", 0, 0); r.Found {
		t.Fatal("harus peka huruf besar/kecil")
	} else if len(r.Similar) == 0 || r.Similar[0] != "docs/a.md" {
		t.Fatalf("similar %v", r.Similar)
	}
	empty := e.project("empty")
	if r := e.read(empty, "x.md", 0, 0); r.Found || len(r.Similar) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestIntegrationLiteralUnderscore(t *testing.T) {
	e := newEnv(t)
	p := e.project("p")
	e.add(p, "a_b/x.md", "benar")
	e.add(p, "axb/x.md", "salah")
	if r := e.read(p, "a_b/x.md", 0, 0); !r.Found || r.Content != "benar" {
		t.Fatalf("%+v", r)
	}
}
