//go:build integration

package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	srUser    = "0192f5a0-aaaa-7aaa-8bbb-0000000000a1"
	srProject = "0192f5a0-aaaa-7aaa-8bbb-0000000000b1"
)

func srSeedProject(t *testing.T, d *DB) {
	t.Helper()
	insUser(t, d, srUser, "sr@x.id")
	if _, err := d.Exec(`INSERT INTO projects (id, name, created_by) VALUES ($1, 'Proj', $2)`, srProject, srUser); err != nil {
		t.Fatalf("project: %v", err)
	}
}

func srInsertFile(t *testing.T, d *DB, path, title, content string, sections string) string {
	t.Helper()
	var id string
	var sec any
	if sections != "" {
		sec = sections
	}
	err := d.Get(&id, `INSERT INTO knowledge_files
  (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label, sections)
VALUES ($1, $2, $3, $4, decode(repeat('ab', 32), 'hex'), $5, $6, 'u', $6, 'u', $7::jsonb) RETURNING id::text`,
		srProject, path, title, content, len(content), srUser, sec)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}
	return id
}

func srBool(t *testing.T, d *DB, q string, args ...any) bool {
	t.Helper()
	var b bool
	if err := d.Get(&b, q, args...); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return b
}

func TestSearchRecallMigration_Columns(t *testing.T) {
	d := migratedDB(t)
	srSeedProject(t, d)
	id := srInsertFile(t, d, "ops/kirim.md", "Pengiriman", "# Pengiriman\nmengirim deployment", "")
	for _, c := range []struct{ name, q string }{
		{"simple", `SELECT search @@ to_tsquery('simple','pengiriman') FROM knowledge_files WHERE id=$1`},
		{"indonesian", `SELECT search_stem @@ (to_tsquery('indonesian','kirim') || to_tsquery('indonesian','irim')) FROM knowledge_files WHERE id=$1`},
		{"english", `SELECT search_stem @@ to_tsquery('english','deploy') FROM knowledge_files WHERE id=$1`},
	} {
		if !srBool(t, d, c.q, id) {
			t.Errorf("%s: tidak cocok", c.name)
		}
	}
}

func TestSearchRecallMigration_Weights(t *testing.T) {
	d := migratedDB(t)
	srSeedProject(t, d)
	a := srInsertFile(t, d, "docs/alpha.md", "Alpha", "isi biasa tentang apa saja", "")
	b := srInsertFile(t, d, "docs/b.md", "B", "alpha ada hanya di isi", "")
	var ra, rb float64
	q := `SELECT ts_rank_cd('{0,0,0,1}'::float4[], search, to_tsquery('simple','alpha'))::float8 FROM knowledge_files WHERE id=$1`
	if err := d.Get(&ra, q, a); err != nil {
		t.Fatal(err)
	}
	if err := d.Get(&rb, q, b); err != nil {
		t.Fatal(err)
	}
	if !(ra > 0) || rb != 0 {
		t.Fatalf("bobot A: judul=%v isi=%v", ra, rb)
	}
	h := srInsertFile(t, d, "docs/h.md", "H", "## Rollback cepat\nteks", `[{"h":"Rollback cepat","s":0,"e":10}]`)
	var rh float64
	if err := d.Get(&rh, `SELECT ts_rank_cd('{0,0,1,0}'::float4[], search, to_tsquery('simple','rollback'))::float8 FROM knowledge_files WHERE id=$1`, h); err != nil {
		t.Fatal(err)
	}
	if !(rh > 0) {
		t.Fatalf("bobot B heading = %v", rh)
	}
}

func srTerms(t *testing.T, d *DB, id string) map[string]bool {
	t.Helper()
	var ts []string
	if err := d.Select(&ts, `SELECT term FROM knowledge_terms WHERE file_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for _, x := range ts {
		m[x] = true
	}
	return m
}

func TestSearchRecallMigration_TermsTrigger(t *testing.T) {
	d := migratedDB(t)
	srSeedProject(t, d)
	id := srInsertFile(t, d, "ops/rollback.md", "Panduan Deployment", "kubernetes kubernetes sekali", "")
	got := srTerms(t, d, id)
	for _, w := range []string{"rollback", "panduan", "deployment", "kubernetes"} {
		if !got[w] {
			t.Errorf("term %q harus ada: %v", w, got)
		}
	}
	if got["sekali"] {
		t.Errorf("lexeme isi yang muncul sekali tidak boleh masuk: %v", got)
	}
	if _, err := d.Exec(`UPDATE knowledge_files SET content='observabilitas observabilitas' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	got = srTerms(t, d, id)
	if !got["observabilitas"] || got["kubernetes"] {
		t.Errorf("update harus mengganti isi tabel: %v", got)
	}
	if _, err := d.Exec(`DELETE FROM knowledge_files WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if got = srTerms(t, d, id); len(got) != 0 {
		t.Errorf("delete harus mengosongkan: %v", got)
	}
}

func TestSearchRecallMigration_OrAgg(t *testing.T) {
	d := migratedDB(t)
	var s string
	if err := d.Get(&s, `SELECT knowledge_tsquery_or_agg(q)::text FROM (VALUES ('a'::tsquery),('b'::tsquery)) v(q)`); err != nil {
		t.Fatal(err)
	}
	if s != `'a' | 'b'` {
		t.Fatalf("agg = %s", s)
	}
}

func TestSearchRecallMigration_Idempotent(t *testing.T) {
	d := migratedDB(t)
	srSeedProject(t, d)
	srInsertFile(t, d, "a.md", "A", "isi isi", "")
	matches, err := filepath.Glob("../../../migrations/project/*_knowledge_search_recall.sql")
	if err != nil || len(matches) != 1 {
		t.Fatalf("glob migrasi: %v %v", matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(context.Background(), string(raw)); err != nil {
		t.Fatalf("jalankan ulang: %v", err)
	}
	if !strings.Contains(string(raw), "pg_trgm") {
		t.Fatal("migrasi harus memasang pg_trgm")
	}
}
