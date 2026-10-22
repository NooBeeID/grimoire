//go:build integration

package getconfig

import (
	"context"
	"testing"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/base/store"
	"grimoire/internal/infra/db"
)

func setup(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, newRepo(store.New(d))
}

func mkProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func mkCfg(t *testing.T, d *db.DB, pid, kind string, result *string) {
	t.Helper()
	_, err := d.ExecContext(context.Background(), `INSERT INTO git_sync_configs
 (project_id, kind, provider, host, repo_url, repo_path, branch, folder, last_status, last_synced_at, last_synced_by_label, last_result)
 VALUES ($1,$2,'github','github.com','https://github.com/a/b','a/b','main','docs','success',now(),'Rina',$3::jsonb)`, pid, kind, result)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRepoGet(t *testing.T) {
	ctx := context.Background()
	d, r := setup(t)
	p, q := mkProject(t, d, "pp"), mkProject(t, d, "qq")

	if c, err := r.Get(ctx, p, base.KindKnowledge); c != nil || err != nil {
		t.Fatalf("kosong: %v %v", c, err)
	}
	if c, err := r.Get(ctx, "abc", base.KindKnowledge); c != nil || err != nil {
		t.Fatalf("bukan uuid: %v %v", c, err)
	}
	good := `{"status":"success","summary":{"created":1,"updated":0,"unchanged":0,"skipped":0,"rejected":0},"items":[],"truncated":false,"error_code":"","error_message":""}`
	mkCfg(t, d, p, "knowledge", &good)
	c, err := r.Get(ctx, p, base.KindKnowledge)
	if err != nil || c == nil || c.RepoPath != "a/b" || c.Branch != "main" || c.Folder != "docs" || c.ProjectID != p ||
		c.LastResult == nil || c.LastResult.Summary.Created != 1 || c.LastSyncedAt == nil || c.LastSyncedAt.Location().String() != "UTC" {
		t.Fatalf("%+v %v", c, err)
	}
	if c, _ := r.Get(ctx, p, base.KindAPISpec); c != nil {
		t.Fatal("kind lain harus nil")
	}
	if c, _ := r.Get(ctx, q, base.KindKnowledge); c != nil {
		t.Fatal("isolasi project")
	}
	mkCfg(t, d, p, "api_spec", nil)
	if c, _ := r.Get(ctx, p, base.KindAPISpec); c == nil || c.LastResult != nil {
		t.Fatalf("null last_result: %+v", c)
	}
	bad := `{"items": 5}`
	mkCfg(t, d, q, "knowledge", &bad)
	if c, err := r.Get(ctx, q, base.KindKnowledge); err != nil || c == nil || c.LastResult != nil {
		t.Fatalf("jsonb rusak: %+v %v", c, err)
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, p); err != nil {
		t.Fatal(err)
	}
	if c, _ := r.Get(ctx, p, base.KindKnowledge); c != nil {
		t.Fatal("cascade")
	}
}
