//go:build integration

package backfillsections

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/db"
	"grimoire/internal/infra/jobs"
)

func TestBackfillSections(t *testing.T) {
	ctx := context.Background()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatal(err)
	}
	var pid string
	if err := d.GetContext(ctx, &pid, `INSERT INTO projects (name) VALUES ('Backfill project') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	ins := func(path, content string) string {
		var id string
		if err := d.GetContext(ctx, &id, `INSERT INTO knowledge_files (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
VALUES ($1,$2,'t',$3,decode(repeat('00',32),'hex'),1,gen_random_uuid(),'x',gen_random_uuid(),'x') RETURNING id::text`, pid, path, content); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a := ins("a.md", "😀\n# Judul\nisi")
	b := ins("b.md", "# B\nx")
	if _, err := d.ExecContext(ctx, `UPDATE knowledge_files SET sections = NULL WHERE project_id = $1`, pid); err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(store.New(d))
	uc := NewUsecase(repo)

	// writer menyela: b diisi sebelum backfill menulis; nilainya tidak boleh ditimpa.
	rows, err := repo.NullSections(ctx, 200)
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %v", rows, err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE knowledge_files SET sections = '[{"h":"writer","s":0,"e":1}]' WHERE id = $1`, b); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSections(ctx, b, []byte(`[]`)); err != nil {
		t.Fatal(err)
	}
	var h string
	if err := d.GetContext(ctx, &h, `SELECT sections->0->>'h' FROM knowledge_files WHERE id = $1`, b); err != nil || h != "writer" {
		t.Fatalf("b ditimpa: %q %v", h, err)
	}

	if err := uc.Run(ctx); err != nil {
		t.Fatal(err)
	}
	var start int
	var heading string
	if err := d.GetContext(ctx, &start, `SELECT (sections->1->>'s')::int FROM knowledge_files WHERE id = $1`, a); err != nil || start != 2 {
		t.Fatalf("offset rune a = %d %v", start, err)
	}
	if err := d.GetContext(ctx, &heading, `SELECT sections->1->>'h' FROM knowledge_files WHERE id = $1`, a); err != nil || heading != "Judul" {
		t.Fatalf("heading a = %q %v", heading, err)
	}
	var left int
	if err := d.GetContext(ctx, &left, `SELECT count(*) FROM knowledge_files WHERE sections IS NULL`); err != nil || left != 0 {
		t.Fatalf("sisa NULL %d %v", left, err)
	}
}

func TestInit_RegistersJob(t *testing.T) {
	d := db.TestDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := app.Deps{DB: d, Log: log, Jobs: jobs.New(d, log)}
	Init(deps)
	defer func() {
		if recover() == nil {
			t.Fatal("Init kedua harus panic (nama duplikat)")
		}
	}()
	Init(deps)
}
