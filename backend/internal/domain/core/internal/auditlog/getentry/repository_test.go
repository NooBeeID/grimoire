//go:build integration

package getentry

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/infra/db"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, slog.Default()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func insert(t *testing.T, d *db.DB, id string, q string, args ...any) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

const insertFull = `INSERT INTO audit_log (id, occurred_at, actor_type, actor_id, actor_label, project_id, project_label,
  action, target_type, target, args, result, status, ip, user_agent, trace_id)
VALUES ($1, $2, 'api_key', $3, 'claude-code-budi (gk_ab12)', $4, 'Payments',
  'mcp.read_file', 'file', 'docs/a.md', $5::jsonb, $6::jsonb, 'ok', $7::inet, 'claude-code/2.1', 'trace1')`

func TestRepository_Get_RoundTrip(t *testing.T) {
	d := testDB(t)
	id, actor, proj := uuid.NewString(), uuid.NewString(), uuid.NewString()
	at := time.Date(2026, 10, 19, 8, 14, 22, 123456000, time.FixedZone("x", 7*3600))
	insert(t, d, id, insertFull, id, at, actor, proj,
		`{"n":1,"b":true,"z":null,"arr":[1,"a"],"o":{"k":{"d":2.5}}}`, `{"bytes":12}`, "203.0.113.5")

	e, found, err := NewRepository(d).Get(context.Background(), id)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if e.ID != id || e.ActorID != actor || e.ProjectID != proj || e.ProjectLabel != "Payments" ||
		e.ActorType != "api_key" || e.ActorLabel != "claude-code-budi (gk_ab12)" || e.Action != "mcp.read_file" ||
		e.TargetType != "file" || e.Target != "docs/a.md" || e.Status != "ok" ||
		e.UserAgent != "claude-code/2.1" || e.TraceID != "trace1" {
		t.Fatalf("field berbeda: %+v", e)
	}
	if e.IP != "203.0.113.5" {
		t.Errorf("ip: %q", e.IP)
	}
	if !e.OccurredAt.Equal(at) || e.OccurredAt.Location() != time.UTC {
		t.Errorf("occurred_at: %v", e.OccurredAt)
	}
	if e.Args["n"] != float64(1) || e.Args["b"] != true {
		t.Errorf("args: %v", e.Args)
	}
	if v, ok := e.Args["z"]; !ok || v != nil {
		t.Errorf("null dalam args hilang: %v", e.Args)
	}
	if arr, _ := e.Args["arr"].([]any); len(arr) != 2 {
		t.Errorf("arr: %v", e.Args["arr"])
	}
	if e.Result["bytes"] != float64(12) {
		t.Errorf("result: %v", e.Result)
	}
}

func TestRepository_Get_IPv6(t *testing.T) {
	d := testDB(t)
	id := uuid.NewString()
	insert(t, d, id, insertFull, id, time.Now(), uuid.NewString(), uuid.NewString(), `{}`, `{}`, "::1")
	e, found, err := NewRepository(d).Get(context.Background(), id)
	if err != nil || !found || e.IP != "::1" {
		t.Fatalf("found=%v err=%v ip=%q", found, err, e.IP)
	}
}

const insertNulls = `INSERT INTO audit_log (id, actor_type, actor_label, action, status)
VALUES ($1, 'anonymous', 'gk_zz99', 'mcp.auth.rejected', 'denied')`

func TestRepository_Get_NullColumns(t *testing.T) {
	d := testDB(t)
	id := uuid.NewString()
	insert(t, d, id, insertNulls, id)
	e, found, err := NewRepository(d).Get(context.Background(), id)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if e.ActorID != "" || e.ProjectID != "" || e.ProjectLabel != "" || e.TargetType != "" ||
		e.Target != "" || e.IP != "" || e.UserAgent != "" {
		t.Fatalf("NULL harus string kosong: %+v", e)
	}
	if e.Args == nil || len(e.Args) != 0 || e.Result == nil || len(e.Result) != 0 {
		t.Fatalf("args/result harus map kosong non-nil: %v %v", e.Args, e.Result)
	}
}

func TestRepository_Get_NotFound(t *testing.T) {
	d := testDB(t)
	_, found, err := NewRepository(d).Get(context.Background(), uuid.NewString())
	if found || err != nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func TestT02_AC3_SnapshotLabelsWithoutReferencedRows(t *testing.T) {
	d := testDB(t)
	id := uuid.NewString()
	insert(t, d, id, insertFull, id, time.Now(), uuid.NewString(), uuid.NewString(), `{}`, `{}`, "203.0.113.5")
	e, found, err := NewRepository(d).Get(context.Background(), id)
	if err != nil || !found || e.ActorLabel != "claude-code-budi (gk_ab12)" || e.ProjectLabel != "Payments" {
		t.Fatalf("found=%v err=%v %+v", found, err, e)
	}
}

func TestT02_AC4_ContentNotReturned(t *testing.T) {
	d := testDB(t)
	id := uuid.NewString()
	insert(t, d, id, insertFull, id, time.Now(), uuid.NewString(), uuid.NewString(), `{}`,
		`{"bytes":12,"content":"[OMITTED]"}`, "203.0.113.5")
	e, _, err := NewRepository(d).Get(context.Background(), id)
	if err != nil || e.Result["content"] != "[OMITTED]" || e.Result["bytes"] != float64(12) || len(e.Result) != 2 {
		t.Fatalf("err=%v result=%v", err, e.Result)
	}
}

func TestRepository_Get_Isolation(t *testing.T) {
	d := testDB(t)
	a, b := uuid.NewString(), uuid.NewString()
	insert(t, d, a, insertFull, a, time.Now(), uuid.NewString(), uuid.NewString(), `{"who":"a"}`, `{}`, "203.0.113.5")
	insert(t, d, b, insertFull, b, time.Now(), uuid.NewString(), uuid.NewString(), `{"who":"b"}`, `{}`, "203.0.113.6")
	e, found, err := NewRepository(d).Get(context.Background(), a)
	if err != nil || !found || e.ID != a || e.Args["who"] != "a" {
		t.Fatalf("found=%v err=%v %+v", found, err, e)
	}
}

func TestRepository_Get_ContextCancelled(t *testing.T) {
	d := testDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, found, err := NewRepository(d).Get(ctx, uuid.NewString())
	if err == nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}
