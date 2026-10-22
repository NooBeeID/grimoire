//go:build integration

package summary

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/db"
)

func setupDB(t testing.TB) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))); err != nil {
		t.Fatal(err)
	}
	return d
}

type seed struct {
	ProjectID, ProjectLabel, Action, Status, Result string
	At                                              time.Time
}

func insert(t testing.TB, d *db.DB, s seed) {
	t.Helper()
	if s.Status == "" {
		s.Status = "ok"
	}
	if s.Result == "" {
		s.Result = "{}"
	}
	var pid, label any
	if s.ProjectID != "" {
		pid = s.ProjectID
	}
	if s.ProjectLabel != "" {
		label = s.ProjectLabel
	}
	_, err := d.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_label, project_id, project_label, action, status, args, result)
		VALUES ($1,$2,'api_key','key',$3::uuid,$4,$5,$6,'{}'::jsonb,$7::jsonb)`,
		uuid.NewString(), s.At, pid, label, s.Action, s.Status, s.Result)
	if err != nil {
		t.Fatal(err)
	}
}

func dur(ms int) string { return fmt.Sprintf(`{"duration_ms":%d}`, ms) }

var t0 = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

func dayWindow() base.Window {
	return base.Window{Range: base.Range7d, From: t0, To: t0.Add(7 * 24 * time.Hour), Bucket: base.BucketDay}
}

func toolMap(a base.Aggregates) map[string]base.ToolAgg {
	m := map[string]base.ToolAgg{}
	for _, x := range a.Tools {
		m[x.Action] = x
	}
	return m
}

func TestT15_AC2_AC3_ToolsProjectsExcludeGateway(t *testing.T) {
	d := setupDB(t)
	A, B := uuid.NewString(), uuid.NewString()
	at := t0.Add(time.Hour)
	for i := 0; i < 5; i++ {
		insert(t, d, seed{ProjectID: A, ProjectLabel: "A", Action: "mcp.read_file", At: at})
	}
	for i := 0; i < 2; i++ {
		insert(t, d, seed{ProjectID: B, ProjectLabel: "B", Action: "mcp.search_knowledge", At: at})
	}
	for _, a := range []string{"mcp.initialize", "mcp.tools_list", "mcp.auth.rejected", "mcp.rate_limited", "mcp.unknown_tool", "auth.login.success"} {
		for i := 0; i < 3; i++ {
			insert(t, d, seed{ProjectID: A, Action: a, Status: "denied", At: at})
		}
	}
	agg, err := NewRepository(d).Load(context.Background(), dayWindow(), "")
	if err != nil {
		t.Fatal(err)
	}
	tm := toolMap(agg)
	if len(tm) != 2 || tm["mcp.read_file"].Calls != 5 || tm["mcp.search_knowledge"].Calls != 2 {
		t.Fatalf("tools %+v", agg.Tools)
	}
	if len(agg.Projects) != 2 || agg.Projects[0].ProjectID != A || agg.Projects[0].Calls != 5 || agg.Projects[1].ProjectID != B || agg.Projects[1].Calls != 2 {
		t.Fatalf("projects %+v", agg.Projects)
	}
}

func TestT15_AC4_LatencyAndErrors(t *testing.T) {
	d := setupDB(t)
	at := t0.Add(time.Hour)
	for i := 1; i <= 10; i++ {
		st := "ok"
		if i == 10 {
			st = "error"
		}
		insert(t, d, seed{Action: "mcp.read_file", Status: st, Result: dur(i * 10), At: at})
	}
	for _, r := range []string{`{}`, `{"duration_ms":"abc"}`, `{"duration_ms":null}`} {
		insert(t, d, seed{Action: "mcp.read_file", Result: r, At: at})
	}
	insert(t, d, seed{Action: "mcp.list_files", Result: `{"duration_ms":"x"}`, At: at})
	agg, err := NewRepository(d).Load(context.Background(), dayWindow(), "")
	if err != nil {
		t.Fatal(err)
	}
	tm := toolMap(agg)
	rf := tm["mcp.read_file"]
	if rf.Calls != 13 || rf.OK != 12 || rf.Errors != 1 {
		t.Fatalf("read_file %+v", rf)
	}
	if rf.P50Ms == nil || *rf.P50Ms != 55 || rf.P95Ms == nil || *rf.P95Ms != 95.5 {
		t.Fatalf("p50/p95 %v %v", rf.P50Ms, rf.P95Ms)
	}
	lf := tm["mcp.list_files"]
	if lf.P50Ms != nil || lf.P95Ms != nil {
		t.Fatalf("list_files latency harus nil: %+v", lf)
	}
}

func TestT15_AC5_TrafficBuckets(t *testing.T) {
	d := setupDB(t)
	from := time.Date(2026, 10, 21, 10, 30, 15, 0, time.UTC)
	w := base.NewWindow(base.Range24h, from.Add(24*time.Hour))
	insert(t, d, seed{Action: "mcp.read_file", At: w.From.Add(time.Minute)})
	insert(t, d, seed{Action: "mcp.read_file", Status: "error", At: w.From.Add(time.Minute)})
	insert(t, d, seed{Action: "mcp.read_file", At: w.To.Add(-time.Second)})
	agg, err := NewRepository(d).Load(context.Background(), w, "")
	if err != nil {
		t.Fatal(err)
	}
	starts := w.BucketStarts()
	if len(starts) != 25 || len(agg.Traffic) != 25 {
		t.Fatalf("bucket %d/%d", len(starts), len(agg.Traffic))
	}
	for i, p := range agg.Traffic {
		if !p.At.Equal(starts[i]) {
			t.Fatalf("bucket %d: %v != %v", i, p.At, starts[i])
		}
	}
	if agg.Traffic[0].Calls != 2 || agg.Traffic[0].Errors != 1 || agg.Traffic[24].Calls != 1 || agg.Traffic[10].Calls != 0 {
		t.Fatalf("traffic %+v", agg.Traffic)
	}
	if _, err := base.BuildReport(w, base.SummaryQuery{Range: base.Range24h}, agg, nil); err != nil {
		t.Fatal(err)
	}
}

func TestT15_AC5_DayBucketCounts(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	for _, tc := range []struct {
		rng  base.Range
		from time.Time
		want int
	}{
		{base.Range7d, t0, 7},
		{base.Range30d, t0, 30},
		{base.Range30d, t0.Add(5 * time.Hour), 31},
	} {
		w := base.NewWindow(tc.rng, tc.from.Add(map[base.Range]time.Duration{base.Range7d: 7 * 24 * time.Hour, base.Range30d: 30 * 24 * time.Hour}[tc.rng]))
		agg, err := r.Load(context.Background(), w, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(agg.Traffic) != tc.want {
			t.Fatalf("%s: %d bucket, ingin %d", tc.rng, len(agg.Traffic), tc.want)
		}
	}
}

func TestT15_AC6_WindowBoundsAndProjectFilter(t *testing.T) {
	d := setupDB(t)
	A, B := uuid.NewString(), uuid.NewString()
	w := dayWindow()
	insert(t, d, seed{ProjectID: A, Action: "mcp.read_file", At: w.From.Add(-time.Microsecond)})
	insert(t, d, seed{ProjectID: A, Action: "mcp.read_file", At: w.From})
	insert(t, d, seed{ProjectID: A, Action: "mcp.read_file", At: w.To})
	insert(t, d, seed{ProjectID: B, Action: "mcp.read_file", At: w.From.Add(time.Hour)})
	r := NewRepository(d)
	all, err := r.Load(context.Background(), w, "")
	if err != nil {
		t.Fatal(err)
	}
	if toolMap(all)["mcp.read_file"].Calls != 2 {
		t.Fatalf("all %+v", all.Tools)
	}
	onlyA, err := r.Load(context.Background(), w, A)
	if err != nil {
		t.Fatal(err)
	}
	if toolMap(onlyA)["mcp.read_file"].Calls != 1 || len(onlyA.Projects) != 1 || onlyA.Projects[0].ProjectID != A {
		t.Fatalf("A %+v %+v", onlyA.Tools, onlyA.Projects)
	}
	sum := 0
	for _, p := range onlyA.Traffic {
		sum += p.Calls
	}
	if sum != 1 {
		t.Fatalf("traffic A = %d", sum)
	}
}

func TestT15_TopProjectsLimitAndTieBreak(t *testing.T) {
	d := setupDB(t)
	at := t0.Add(time.Hour)
	for i := 0; i < 12; i++ {
		pid := fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
		n := 1 + i/2 // pasangan seri
		for j := 0; j < n; j++ {
			insert(t, d, seed{ProjectID: pid, Action: "mcp.list_files", At: at})
		}
	}
	agg, err := NewRepository(d).Load(context.Background(), dayWindow(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(agg.Projects) != 10 {
		t.Fatalf("len %d", len(agg.Projects))
	}
	for i := 1; i < len(agg.Projects); i++ {
		p, c := agg.Projects[i-1], agg.Projects[i]
		if p.Calls < c.Calls || (p.Calls == c.Calls && p.ProjectID > c.ProjectID) {
			t.Fatalf("urutan salah di %d: %+v %+v", i, p, c)
		}
	}
}

func TestT15_ProjectLabelLatest(t *testing.T) {
	d := setupDB(t)
	P, Q := uuid.NewString(), uuid.NewString()
	insert(t, d, seed{ProjectID: P, ProjectLabel: "Payments", Action: "mcp.list_files", At: t0.Add(time.Hour)})
	insert(t, d, seed{ProjectID: P, ProjectLabel: "Payments v2", Action: "mcp.list_files", At: t0.Add(2 * time.Hour)})
	insert(t, d, seed{ProjectID: Q, Action: "mcp.list_files", At: t0.Add(time.Hour)})
	agg, err := NewRepository(d).Load(context.Background(), dayWindow(), "")
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{}
	for _, p := range agg.Projects {
		labels[p.ProjectID] = p.ProjectLabel
	}
	if labels[P] != "Payments v2" || labels[Q] != "" {
		t.Fatalf("labels %v", labels)
	}
}

func TestT15_EmptyAndCleanTx(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	agg, err := r.Load(context.Background(), dayWindow(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if len(agg.Tools) != 0 || len(agg.Projects) != 0 || len(agg.Traffic) != 7 {
		t.Fatalf("agg %+v", agg)
	}
	for _, p := range agg.Traffic {
		if p.Calls != 0 || p.Errors != 0 {
			t.Fatalf("traffic tidak nol: %+v", p)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Load(ctx, dayWindow(), ""); err == nil {
		t.Fatal("ctx batal harus error")
	}
	if n := d.Stats().InUse; n != 0 {
		t.Fatalf("koneksi tertahan: %d", n)
	}
}
