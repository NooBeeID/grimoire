//go:build integration

package scanentries

import (
	"bytes"
	"context"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/infra/db"
	"grimoire/lib/contracts/core/audit/scanentries"
)

func setupDB(t testing.TB) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))); err != nil {
		t.Fatal(err)
	}
	return d
}

type seedRow struct {
	ID, Project, Action, Target, ActorID, Args, Result string
	At                                                 time.Time
}

func seed(t testing.TB, d *db.DB, r seedRow) string {
	t.Helper()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.Action == "" {
		r.Action = "mcp.search_knowledge"
	}
	if r.Args == "" {
		r.Args = "{}"
	}
	if r.Result == "" {
		r.Result = "{}"
	}
	nul := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	_, err := d.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_id, actor_label, project_id, action, target, status, args, result)
		VALUES ($1,$2,'system',$3::uuid,'system',$4::uuid,$5,$6,'ok',$7::jsonb,$8::jsonb)`,
		r.ID, r.At, nul(r.ActorID), nul(r.Project), r.Action, nul(r.Target), r.Args, r.Result)
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func baseReq(project string) scanentries.Request {
	return scanentries.Request{
		ProjectID: project,
		Actions:   []string{"mcp.search_knowledge", "mcp.read_file"},
		From:      t0,
		To:        t0.Add(24 * time.Hour),
	}
}

func idsOf(es []scanentries.Entry) map[string]bool {
	m := map[string]bool{}
	for _, e := range es {
		m[e.ID] = true
	}
	return m
}

func TestScan_OnlyRequestedProject(t *testing.T) {
	d := setupDB(t)
	a, b := uuid.NewString(), uuid.NewString()
	ia := seed(t, d, seedRow{Project: a, At: t0.Add(time.Hour)})
	seed(t, d, seedRow{Project: b, At: t0.Add(time.Hour)})
	seed(t, d, seedRow{At: t0.Add(time.Hour)})
	resp, err := NewUsecase(NewRepository(d), time.Now).Execute(context.Background(), baseReq(a))
	if err != nil || len(resp.Entries) != 1 || resp.Entries[0].ID != ia {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestScan_OnlyRequestedActions(t *testing.T) {
	d := setupDB(t)
	p := uuid.NewString()
	seed(t, d, seedRow{Project: p, Action: "mcp.search_knowledge", At: t0.Add(time.Hour)})
	seed(t, d, seedRow{Project: p, Action: "mcp.read_file", At: t0.Add(time.Hour)})
	seed(t, d, seedRow{Project: p, Action: "mcp.list_files", At: t0.Add(time.Hour)})
	resp, err := NewUsecase(NewRepository(d), time.Now).Execute(context.Background(), baseReq(p))
	if err != nil || len(resp.Entries) != 2 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	for _, e := range resp.Entries {
		if e.Action == "mcp.list_files" {
			t.Fatal("leaked action")
		}
	}
}

func TestScan_TimeBounds(t *testing.T) {
	d := setupDB(t)
	p := uuid.NewString()
	to := t0.Add(24 * time.Hour)
	atFrom := seed(t, d, seedRow{Project: p, At: t0})
	seed(t, d, seedRow{Project: p, At: to})
	seed(t, d, seedRow{Project: p, At: t0.Add(-time.Microsecond)})
	beforeTo := seed(t, d, seedRow{Project: p, At: to.Add(-time.Microsecond)})
	resp, err := NewUsecase(NewRepository(d), time.Now).Execute(context.Background(), baseReq(p))
	if err != nil {
		t.Fatal(err)
	}
	got := idsOf(resp.Entries)
	if len(got) != 2 || !got[atFrom] || !got[beforeTo] {
		t.Fatalf("got=%v", got)
	}
}

func TestScan_PagingIdenticalTimestamps(t *testing.T) {
	d := setupDB(t)
	p := uuid.NewString()
	want := map[string]bool{}
	same := t0.Add(time.Hour)
	for i := 0; i < 7; i++ {
		want[seed(t, d, seedRow{Project: p, At: same})] = true
	}
	want[seed(t, d, seedRow{Project: p, At: t0.Add(time.Minute)})] = true
	want[seed(t, d, seedRow{Project: p, At: t0.Add(2 * time.Hour)})] = true

	uc := NewUsecase(NewRepository(d), time.Now)
	in := baseReq(p)
	in.Limit = 3
	var all []scanentries.Entry
	var sizes []int
	for i := 0; i < 10; i++ {
		resp, err := uc.Execute(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(resp.Entries))
		all = append(all, resp.Entries...)
		if resp.Next == nil {
			break
		}
		in.After = resp.Next
	}
	if len(sizes) != 3 || sizes[0] != 3 || sizes[1] != 3 || sizes[2] != 3 {
		t.Fatalf("sizes=%v", sizes)
	}
	got := idsOf(all)
	if len(all) != 9 || len(got) != 9 {
		t.Fatalf("all=%d unique=%d", len(all), len(got))
	}
	for id := range want {
		if !got[id] {
			t.Fatalf("missing %s", id)
		}
	}
	if !sort.SliceIsSorted(all, func(i, j int) bool {
		if !all[i].OccurredAt.Equal(all[j].OccurredAt) {
			return all[i].OccurredAt.Before(all[j].OccurredAt)
		}
		return all[i].ID < all[j].ID
	}) {
		t.Fatal("not sorted")
	}
}

func TestScan_Content(t *testing.T) {
	d := setupDB(t)
	p := uuid.NewString()
	seed(t, d, seedRow{
		Project: p, At: t0.Add(time.Hour), Target: "docs", ActorID: uuid.NewString(),
		Args:   `{"limit":10,"nested":{"k":"v"}}`,
		Result: `{"count":3,"paths":["a","b"]}`,
	})
	seed(t, d, seedRow{Project: p, At: t0.Add(2 * time.Hour)})
	resp, err := NewUsecase(NewRepository(d), time.Now).Execute(context.Background(), baseReq(p))
	if err != nil || len(resp.Entries) != 2 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	e1, e2 := resp.Entries[0], resp.Entries[1]
	if e1.Args["limit"] != float64(10) || e1.Args["nested"].(map[string]any)["k"] != "v" {
		t.Fatalf("args=%v", e1.Args)
	}
	if _, ok := e1.Result["paths"].([]any); !ok || e1.Result["count"] != float64(3) {
		t.Fatalf("result=%v", e1.Result)
	}
	if e1.Target != "docs" || e1.ActorID == "" || e1.OccurredAt.Location() != time.UTC {
		t.Fatalf("e1=%+v", e1)
	}
	if e2.Args == nil || len(e2.Args) != 0 || e2.Result == nil || len(e2.Result) != 0 || e2.Target != "" || e2.ActorID != "" {
		t.Fatalf("e2=%+v", e2)
	}
}

func TestScan_EmptyAndUnknownProject(t *testing.T) {
	d := setupDB(t)
	resp, err := NewUsecase(NewRepository(d), time.Now).Execute(context.Background(), baseReq(uuid.NewString()))
	if err != nil || resp.Entries == nil || len(resp.Entries) != 0 || resp.Next != nil {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestScan_WeirdActionIsInert(t *testing.T) {
	d := setupDB(t)
	p := uuid.NewString()
	seed(t, d, seedRow{Project: p, At: t0.Add(time.Hour)})
	rows, err := NewRepository(d).Scan(context.Background(), params{
		ProjectID: p, Actions: []string{"x'; DROP TABLE audit_log; --"}, From: t0, To: t0.Add(24 * time.Hour), Limit: 10,
	}, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	var n int
	if err := d.Get(&n, `SELECT count(*) FROM audit_log`); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
