//go:build integration

package runsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/base/audit"
	"grimoire/internal/domain/project/internal/gitsync/base/store"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/db"
	"grimoire/internal/infra/gitprovider"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/common/secret"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
	"grimoire/lib/contracts/project/management/getproject"
)

func setupRepo(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, newRepo(d, store.New(d))
}

func mkProj(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func cfgFor(pid string, k base.Kind, branch, prefix string) base.Config {
	return base.Config{ProjectID: pid, Kind: k, Provider: "github", Host: "github.com", RepoURL: "https://github.com/a/b",
		RepoPath: "a/b", Branch: branch, Folder: "docs", TargetPrefix: prefix}
}

func TestUpsertConfig(t *testing.T) {
	ctx := context.Background()
	d, r := setupRepo(t)
	p := mkProj(t, d, "pp")

	c1, err := r.UpsertConfig(ctx, cfgFor(p, base.KindKnowledge, "main", ""))
	if err != nil || c1.Branch != "main" {
		t.Fatalf("%v %+v", err, c1)
	}
	time.Sleep(5 * time.Millisecond)
	c2, err := r.UpsertConfig(ctx, cfgFor(p, base.KindKnowledge, "dev", "guides"))
	if err != nil || c2.Branch != "dev" || c2.TargetPrefix != "guides" || c2.ID != c1.ID || !c2.CreatedAt.Equal(c1.CreatedAt) || !c2.UpdatedAt.After(c1.UpdatedAt) || c2.LastStatus != nil {
		t.Fatalf("%v %+v", err, c2)
	}
	if _, err := r.UpsertConfig(ctx, cfgFor(p, base.KindAPISpec, "main", "")); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.GetContext(ctx, &n, `SELECT count(*) FROM git_sync_configs WHERE project_id=$1`, p)
	if n != 2 {
		t.Fatalf("rows=%d", n)
	}
	if _, err := r.UpsertConfig(ctx, cfgFor("00000000-0000-0000-0000-000000000000", base.KindKnowledge, "main", "")); !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("%v", err)
	}
	if _, err := r.UpsertConfig(ctx, cfgFor(p, base.KindAPISpec, "main", "x")); err == nil {
		t.Fatal("prefix api_spec harus ditolak constraint")
	}
}

func TestSaveResult(t *testing.T) {
	ctx := context.Background()
	d, r := setupRepo(t)
	p := mkProj(t, d, "pp")
	if _, err := r.SaveResult(ctx, SaveResultParams{ProjectID: p, Kind: base.KindKnowledge, Status: "success"}); !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("%v", err)
	}
	if _, err := r.UpsertConfig(ctx, cfgFor(p, base.KindKnowledge, "main", "")); err != nil {
		t.Fatal(err)
	}
	rep := base.NewReport([]base.ReportItem{{RepoPath: "docs/a.md", Target: "a.md", Status: base.ItemCreated, Issues: []base.Issue{}}})
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	c, err := r.SaveResult(ctx, SaveResultParams{ProjectID: p, Kind: base.KindKnowledge, Status: "success", SyncedAt: now,
		ByID: "0192f5a0-1111-7aaa-8bbb-000000000001", ByLabel: "Rina", Result: rep})
	if err != nil || c.LastStatus == nil || *c.LastStatus != "success" || c.LastSyncedBy == nil || *c.LastSyncedBy != "0192f5a0-1111-7aaa-8bbb-000000000001" ||
		*c.LastSyncedByLabel != "Rina" || c.LastResult == nil || c.LastResult.Summary.Created != 1 || !c.LastSyncedAt.Equal(now) {
		t.Fatalf("%v %+v", err, c)
	}
	c, err = r.SaveResult(ctx, SaveResultParams{ProjectID: p, Kind: base.KindKnowledge, Status: "failed", SyncedAt: now,
		ByID: "0192f5a0-1111-7aaa-8bbb-000000000001", ByLabel: "Rina", Result: base.FailedReport("TIMEOUT", "x")})
	if err != nil || c.LastResult.ErrorCode != "TIMEOUT" || *c.LastStatus != "failed" {
		t.Fatalf("%v %+v", err, c)
	}
}

func TestTryLock(t *testing.T) {
	ctx := context.Background()
	d, r := setupRepo(t)
	p, q := mkProj(t, d, "pp"), mkProj(t, d, "qq")

	unlock, ok, err := r.TryLock(ctx, p, base.KindKnowledge)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if _, ok2, _ := r.TryLock(ctx, p, base.KindKnowledge); ok2 {
		t.Fatal("lock ganda")
	}
	u2, ok3, _ := r.TryLock(ctx, p, base.KindAPISpec)
	u3, ok4, _ := r.TryLock(ctx, q, base.KindKnowledge)
	if !ok3 || !ok4 {
		t.Fatal("kunci berbeda tidak boleh bentrok")
	}
	_ = u2(ctx)
	_ = u3(ctx)
	if err := unlock(ctx); err != nil {
		t.Fatal(err)
	}
	u4, ok5, _ := r.TryLock(ctx, p, base.KindKnowledge)
	if !ok5 {
		t.Fatal("lock harus lepas setelah unlock")
	}
	_ = u4(ctx)
	_ = unlock(ctx) // kedua kali tidak panik

	if s := d.Stats(); s.InUse != 0 {
		t.Fatalf("koneksi bocor: %+v", s)
	}
}

func TestTryLockReleasedWhenConnectionDropped(t *testing.T) {
	ctx := context.Background()
	d, r := setupRepo(t)
	p := mkProj(t, d, "pp")
	unlock, ok, _ := r.TryLock(ctx, p, base.KindKnowledge)
	if !ok {
		t.Fatal()
	}
	// unlock dengan ctx yang sudah mati: koneksi dibuang sehingga lock lepas.
	dead, cancel := context.WithCancel(ctx)
	cancel()
	if err := unlock(dead); err == nil {
		t.Fatal("harus error")
	}
	u, ok2, err := r.TryLock(ctx, p, base.KindKnowledge)
	if err != nil || !ok2 {
		t.Fatalf("lock harus lepas: %v %v", ok2, err)
	}
	_ = u(ctx)
}

// --- Canary: usecase penuh di atas Postgres nyata; audit_log dan git_sync_configs.last_result
// tidak boleh memuat PAT, termasuk saat error upstream membawa PAT di pesan.

const syncActor = "0192f5a0-1111-7aaa-8bbb-000000000001"

// auditToDB meniru provider core.audit.record: menulis Request apa adanya ke tabel audit_log.
func auditToDB(d *db.DB) bus.Handler {
	return func(ctx context.Context, req any) (any, error) {
		r, err := bus.Expect[record.Request](req)
		if err != nil {
			return nil, err
		}
		args, _ := json.Marshal(r.Args)
		result, _ := json.Marshal(r.Result)
		_, err = d.ExecContext(ctx, `INSERT INTO audit_log
(actor_type, actor_id, actor_label, project_id, project_label, action, target_type, target, args, status, result, ip, user_agent, trace_id)
VALUES ($1,$2::uuid,$3,$4::uuid,$5,$6,$7,$8,$9::jsonb,$10,$11::jsonb,NULLIF($12,'')::inet,$13,$14)`,
			r.ActorType, r.ActorID, r.ActorLabel, r.ProjectID, r.ProjectLabel, r.Action, r.TargetType, r.Target,
			args, r.Status, result, r.IP, r.UserAgent, r.TraceID)
		return record.Response{}, err
	}
}

func newCanaryUsecase(t *testing.T, d *db.DB, e *env, projectName string) *Usecase {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(e.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	b := bus.NewRegistry(log)
	b.Register(constants.BUS_CORE_AUDIT_RECORD, auditToDB(d))
	b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(context.Context, any) (any, error) {
		return getproject.Response{Found: true, Name: projectName}, nil
	})
	b.Register(constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, func(context.Context, any) (any, error) {
		return gettoken.Response{Found: true, Token: secret.New(canary)}, nil
	})
	b.Register(constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, func(_ context.Context, req any) (any, error) {
		r, err := bus.Expect[upsertfiles.Request](req)
		if err != nil {
			return nil, err
		}
		res := upsertfiles.Response{}
		for _, it := range r.Items {
			res.Items = append(res.Items, upsertfiles.ItemResult{Index: it.Index, Path: it.Path, Status: "created"})
		}
		return res, nil
	})
	e.acc.actor = base.Actor{ID: syncActor, Role: "developer", Label: "Rina"}
	cfg := config.Config{MaxFileBytes: 1000, MaxUploadFiles: 50, MaxUploadBytes: 10000, APISpecMaxFileBytes: 1000, APISpecMaxSpecs: 50}
	clock := func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }
	return NewUsecase(e.acc, newRepo(d, store.New(d)), e.git, b, audit.New(b, log), clock, cfg, log)
}

func TestCanary_NotPersistedInAuditOrLastResult(t *testing.T) {
	scenarios := []struct {
		name      string
		setup     func(*env)
		wantAudit string
		wantSt    string
	}{
		{"success", func(e *env) {}, "ok", "success"},
		{"token ditolak", func(e *env) { e.git.treeErr = gitprovider.ErrUnauthorized }, "error", "failed"},
		{"upstream", func(e *env) { e.git.treeErr = gitprovider.ErrUpstream }, "error", "failed"},
		{"error tak dikenal membawa PAT", func(e *env) { e.git.treeErr = fmt.Errorf("dial https://x:%s@github.com: boom", canary) }, "error", "failed"},
		{"blob error membawa PAT", func(e *env) { e.git.blobErr = fmt.Errorf("fetch with %s failed", canary) }, "error", ""},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: syncActor, Role: "developer", Name: "Rina"})
			d := db.TestDB(t)
			if _, err := db.Up(context.Background(), d, nil); err != nil {
				t.Fatal(err)
			}
			p := mkProj(t, d, "proj-canary")
			e := newEnv(t)
			e.addFile("docs/a.md", "isi")
			sc.setup(e)
			uc := newCanaryUsecase(t, d, e, "proj-canary")

			res, err := uc.Execute(ctx, p, kreq(), base.RequestMeta{IP: "10.0.0.1", UserAgent: "ua"})
			blob := fmt.Sprintf("%v|%+v|", err, res)
			if j, jerr := json.Marshal(res); jerr == nil {
				blob += string(j)
			}

			var auditRows []string
			if err := d.Select(&auditRows, `SELECT (audit_log.*)::text FROM audit_log WHERE action='git_sync.run'`); err != nil {
				t.Fatal(err)
			}
			if len(auditRows) != 1 {
				t.Fatalf("audit git_sync.run: %d baris, want 1", len(auditRows))
			}
			var auditStatus string
			if err := d.Get(&auditStatus, `SELECT status FROM audit_log WHERE action='git_sync.run'`); err != nil || auditStatus != sc.wantAudit {
				t.Fatalf("status audit %q err=%v want %q", auditStatus, err, sc.wantAudit)
			}
			var lastResult, lastStatus *string
			if err := d.QueryRow(`SELECT last_result::text, last_status FROM git_sync_configs WHERE project_id=$1 AND kind='knowledge'`, p).
				Scan(&lastResult, &lastStatus); err != nil {
				t.Fatal(err)
			}
			if lastResult == nil || lastStatus == nil {
				t.Fatal("last_result/last_status harus tersimpan")
			}
			if sc.wantSt != "" && *lastStatus != sc.wantSt {
				t.Fatalf("last_status=%q want %q", *lastStatus, sc.wantSt)
			}

			all := blob + "|" + auditRows[0] + "|" + *lastResult
			for _, marker := range []string{"CANARY", canary} {
				if strings.Contains(all, marker) {
					t.Fatalf("PAT bocor ke hasil/audit_log/last_result: %s", marker)
				}
			}
		})
	}
}
