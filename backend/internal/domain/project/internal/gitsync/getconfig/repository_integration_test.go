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

func holdLock(t *testing.T, d *db.DB, projectID string, kind base.Kind) (release func()) {
	t.Helper()
	ctx := context.Background()
	conn, err := d.Connx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock("+store.LockKeySQL+")", projectID, string(kind)); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	done := false
	release = func() {
		if done {
			return
		}
		done = true
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock("+store.LockKeySQL+")", projectID, string(kind))
		conn.Close()
	}
	t.Cleanup(release)
	return release
}

func TestRepoInProgress(t *testing.T) {
	ctx := context.Background()
	d, r := setup(t)
	p, q := mkProject(t, d, "pp"), mkProject(t, d, "qq")

	if ok, err := r.InProgress(ctx, p, base.KindKnowledge); ok || err != nil {
		t.Fatalf("tanpa lock: %v %v", ok, err)
	}
	release := holdLock(t, d, p, base.KindKnowledge)
	if ok, err := r.InProgress(ctx, p, base.KindKnowledge); !ok || err != nil {
		t.Fatalf("lock dipegang: %v %v", ok, err)
	}
	if ok, _ := r.InProgress(ctx, p, base.KindAPISpec); ok {
		t.Fatal("kind lain harus false")
	}
	if ok, _ := r.InProgress(ctx, q, base.KindKnowledge); ok {
		t.Fatal("project lain harus false")
	}
	// IsLocked tidak meninggalkan lock: koneksi lain tetap bisa TryLock setelah pemegang lepas.
	release()
	if ok, _ := r.InProgress(ctx, p, base.KindKnowledge); ok {
		t.Fatal("setelah release harus false")
	}
	var got bool
	if err := d.GetContext(ctx, &got, "SELECT pg_try_advisory_lock("+store.LockKeySQL+")", p, string(base.KindKnowledge)); err != nil || !got {
		t.Fatalf("try lock: %v %v", got, err)
	}
	_, _ = d.ExecContext(ctx, "SELECT pg_advisory_unlock_all()")
	if ok, err := r.InProgress(ctx, "abc", base.KindKnowledge); ok || err != nil {
		t.Fatalf("bukan uuid: %v %v", ok, err)
	}
}

// Hash negatif dan positif harus sama-sama terdeteksi (sign-extension classid).
func TestRepoInProgressHashSign(t *testing.T) {
	ctx := context.Background()
	d, r := setup(t)
	var neg, pos string
	for i := 0; i < 64 && (neg == "" || pos == ""); i++ {
		var id string
		var isNeg bool
		if err := d.QueryRowxContext(ctx, `WITH g AS (SELECT gen_random_uuid()::text AS id)
SELECT id, (hashtext('grimoire.gitsync.' || id || '.knowledge'))::bigint < 0 FROM g`).Scan(&id, &isNeg); err != nil {
			t.Fatal(err)
		}
		if isNeg && neg == "" {
			neg = id
		} else if !isNeg && pos == "" {
			pos = id
		}
	}
	if neg == "" || pos == "" {
		t.Fatal("tidak menemukan kedua tanda hash")
	}
	for _, id := range []string{neg, pos} {
		release := holdLock(t, d, id, base.KindKnowledge)
		if ok, err := r.InProgress(ctx, id, base.KindKnowledge); !ok || err != nil {
			t.Fatalf("%s: %v %v", id, ok, err)
		}
		release()
		if ok, _ := r.InProgress(ctx, id, base.KindKnowledge); ok {
			t.Fatalf("%s setelah release", id)
		}
	}
}
