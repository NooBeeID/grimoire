//go:build integration

package usage

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"reflect"
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
	ProjectID, ActorType, ActorID, Label, Action, Status, Target, Args, Result string
	At                                                                         time.Time
}

func insert(t testing.TB, d *db.DB, s seed) {
	t.Helper()
	if s.Status == "" {
		s.Status = "ok"
	}
	if s.ActorType == "" {
		s.ActorType = "api_key"
	}
	if s.Label == "" {
		s.Label = "key"
	}
	if s.Args == "" {
		s.Args = "{}"
	}
	if s.Result == "" {
		s.Result = "{}"
	}
	var pid, aid, target any
	if s.ProjectID != "" {
		pid = s.ProjectID
	}
	if s.ActorID != "" {
		aid = s.ActorID
	}
	if s.Target != "" {
		target = s.Target
	}
	_, err := d.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_id, actor_label, project_id, action, target, status, args, result)
		VALUES ($1,$2,$3,$4::uuid,$5,$6::uuid,$7,$8,$9,$10::jsonb,$11::jsonb)`,
		uuid.NewString(), s.At, s.ActorType, aid, s.Label, pid, s.Action, target, s.Status, s.Args, s.Result)
	if err != nil {
		t.Fatal(err)
	}
}

var (
	t0   = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	win  = base.Window{From: t0, To: t0.Add(7 * 24 * time.Hour)}
	P    = uuid.NewString()
	P2   = uuid.NewString()
	K1   = uuid.NewString()
	K2   = uuid.NewString()
	ctx  = context.Background()
	hour = time.Hour
)

func read(t testing.TB, d *db.DB, pid, key, action, target, status string, at time.Time) {
	insert(t, d, seed{ProjectID: pid, ActorID: key, Action: action, Target: target, Status: status, At: at})
}

func zq(t testing.TB, d *db.DB, pid, key, query, result string, at time.Time) {
	insert(t, d, seed{ProjectID: pid, ActorID: key, Action: "mcp.search_knowledge", Args: fmt.Sprintf(`{"query":%q}`, query), Result: result, At: at})
}

func TestT19_AC1_DuplicateReadsOneKey(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	for i := 0; i < 3; i++ {
		read(t, d, P, K1, "mcp.read_file", "a.md", "ok", t0.Add(time.Duration(i+1)*hour))
	}
	got, err := r.TopFiles(ctx, win, P, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := []base.FileRead{{Path: "a.md", Reads: 3, Keys: 1, LastReadAt: t0.Add(3 * hour)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestT19_AC1_ReadSectionCounted(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	read(t, d, P, K1, "mcp.read_file", "a.md", "ok", t0.Add(hour))
	read(t, d, P, K2, "mcp.read_section", "a.md", "ok", t0.Add(2*hour))
	read(t, d, P, K2, "mcp.read_section", "a.md", "ok", t0.Add(3*hour))
	top, err := r.TopFiles(ctx, win, P, 20)
	if err != nil || len(top) != 1 || top[0].Reads != 3 || top[0].Keys != 2 {
		t.Fatalf("top=%+v err=%v", top, err)
	}
	un, err := r.UnreadPaths(ctx, win, P, []string{"a.md", "b.md"}, 51)
	if err != nil || !reflect.DeepEqual(un, []string{"b.md"}) {
		t.Fatalf("unread=%v err=%v", un, err)
	}
}

func TestT19_AC1_QueryCaseInsensitiveOneRow(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	zq(t, d, P, K1, "Rate Limit", `{"total":0}`, t0.Add(hour))
	zq(t, d, P, K2, "rate limit", `{"total":0}`, t0.Add(2*hour))
	zq(t, d, P, K1, " RATE limit ", `{"total":0}`, t0.Add(3*hour))
	got, err := r.ZeroQueries(ctx, win, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	want := []base.ZeroQuery{{Query: "rate limit", Count: 3, Keys: 2, LastAt: t0.Add(3 * hour)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestT19_ZeroQueriesNormalizeWhitespace(t *testing.T) {
	d := setupDB(t)
	zq(t, d, P, K1, "rate\tlimit", `{"total":0}`, t0.Add(hour))
	zq(t, d, P, K1, "rate\n limit", `{"total":0}`, t0.Add(2*hour))
	zq(t, d, P, K1, "rate    limit", `{"total":0}`, t0.Add(3*hour))
	got, err := NewRepository(d).ZeroQueries(ctx, win, P, 20)
	if err != nil || len(got) != 1 || got[0].Query != "rate limit" || got[0].Count != 3 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestT19_ZeroQueriesIgnoresNonNumericTotal(t *testing.T) {
	d := setupDB(t)
	zq(t, d, P, K1, "a", `{}`, t0.Add(hour))
	zq(t, d, P, K1, "b", `{"total":"0"}`, t0.Add(hour))
	zq(t, d, P, K1, "c", `{"total":2}`, t0.Add(hour))
	zq(t, d, P, K1, "   ", `{"total":0}`, t0.Add(hour))
	insert(t, d, seed{ProjectID: P, ActorID: K1, Action: "mcp.search_knowledge", Args: `{"query":5}`, Result: `{"total":0}`, At: t0.Add(hour)})
	insert(t, d, seed{ProjectID: P, ActorID: K1, Action: "mcp.search_knowledge", Args: `{}`, Result: `{"total":0}`, At: t0.Add(hour)})
	got, err := NewRepository(d).ZeroQueries(ctx, win, "", 20)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestT19_ErrorReadsIgnored(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	read(t, d, P, K1, "mcp.read_file", "b.md", "error", t0.Add(hour))
	read(t, d, P, K1, "mcp.read_file", "c.md", "denied", t0.Add(hour))
	top, err := r.TopFiles(ctx, win, P, 20)
	if err != nil || len(top) != 0 {
		t.Fatalf("top=%+v err=%v", top, err)
	}
	un, err := r.UnreadPaths(ctx, win, P, []string{"b.md", "c.md"}, 51)
	if err != nil || !reflect.DeepEqual(un, []string{"b.md", "c.md"}) {
		t.Fatalf("unread=%v err=%v", un, err)
	}
}

func TestT19_WindowAndProjectIsolation(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	read(t, d, P, K1, "mcp.read_file", "a.md", "ok", t0.Add(hour))
	read(t, d, P2, K1, "mcp.read_file", "a.md", "ok", t0.Add(hour))
	read(t, d, P, K1, "mcp.read_file", "a.md", "ok", t0.Add(-hour)) // sebelum from
	read(t, d, P, K1, "mcp.read_file", "a.md", "ok", win.To)        // to eksklusif
	zq(t, d, P2, K1, "x", `{"total":0}`, t0.Add(hour))
	zq(t, d, P, K1, "x", `{"total":0}`, win.To)

	top, _ := r.TopFiles(ctx, win, P, 20)
	if len(top) != 1 || top[0].Reads != 1 {
		t.Fatalf("top %+v", top)
	}
	un, _ := r.UnreadPaths(ctx, win, P2, []string{"a.md"}, 51)
	if len(un) != 0 {
		t.Fatalf("unread P2 %v", un)
	}
	un, _ = r.UnreadPaths(ctx, win, P, []string{"a.md"}, 51)
	if len(un) != 0 {
		t.Fatalf("unread P %v", un)
	}
	zp, _ := r.ZeroQueries(ctx, win, P, 20)
	if len(zp) != 0 {
		t.Fatalf("zero P %+v", zp)
	}
	za, _ := r.ZeroQueries(ctx, win, "", 20)
	if len(za) != 1 || za[0].Count != 1 {
		t.Fatalf("zero all %+v", za)
	}
	kp, _ := r.PerKey(ctx, win, P, 20)
	if len(kp) != 1 || kp[0].Calls != 1 {
		t.Fatalf("perkey P %+v", kp)
	}
}

func TestT19_OrderingAndLimit(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	for i := 0; i < 25; i++ {
		read(t, d, P, K1, "mcp.read_file", fmt.Sprintf("f%02d.md", i), "ok", t0.Add(hour))
	}
	read(t, d, P, K1, "mcp.read_file", "a.md", "ok", t0.Add(hour))
	read(t, d, P, K1, "mcp.read_file", "A.md", "ok", t0.Add(hour))
	read(t, d, P, K1, "mcp.read_file", "top.md", "ok", t0.Add(hour))
	read(t, d, P, K1, "mcp.read_file", "top.md", "ok", t0.Add(hour))
	got, err := r.TopFiles(ctx, win, P, 20)
	if err != nil || len(got) != 20 {
		t.Fatalf("len=%d err=%v", len(got), err)
	}
	if got[0].Path != "top.md" || got[1].Path != "A.md" || got[2].Path != "a.md" || got[3].Path != "f00.md" {
		t.Fatalf("urutan: %v %v %v %v", got[0].Path, got[1].Path, got[2].Path, got[3].Path)
	}
}

func TestT19_PerKeyCounts(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	insert(t, d, seed{ProjectID: P, ActorID: K1, Label: "old", Action: "mcp.read_file", Target: "a.md", At: t0.Add(1 * hour)})
	insert(t, d, seed{ProjectID: P, ActorID: K1, Label: "ci-bot", Action: "mcp.read_file", Target: "b.md", At: t0.Add(5 * hour)})
	insert(t, d, seed{ProjectID: P, ActorID: K1, Label: "ci-bot", Action: "mcp.read_file", Status: "error", Target: "x.md", At: t0.Add(3 * hour)})
	insert(t, d, seed{ProjectID: P, ActorID: K1, Label: "ci-bot", Action: "mcp.search_knowledge", At: t0.Add(4 * hour)})
	insert(t, d, seed{ProjectID: P, ActorID: K1, Label: "ci-bot", Action: "mcp.initialize", At: t0.Add(4 * hour)})
	insert(t, d, seed{ProjectID: P, ActorType: "user", ActorID: K2, Action: "mcp.read_file", At: t0.Add(4 * hour)})
	got, err := r.PerKey(ctx, win, "", 20)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	x := got[0]
	want := base.KeyUsage{KeyID: K1, Label: "ci-bot", Calls: 4, Errors: 1, Searches: 1, Reads: 2, LastUsedAt: t0.Add(5 * hour), TopTool: "mcp.read_file"}
	if x != want {
		t.Fatalf("got %+v want %+v", x, want)
	}
}

func TestT19_PerKeyTopToolTieBreak(t *testing.T) {
	d := setupDB(t)
	for i := 0; i < 2; i++ {
		insert(t, d, seed{ProjectID: P, ActorID: K1, Action: "mcp.read_file", Target: "a.md", At: t0.Add(hour)})
		insert(t, d, seed{ProjectID: P, ActorID: K1, Action: "mcp.list_files", At: t0.Add(hour)})
	}
	got, err := NewRepository(d).PerKey(ctx, win, P, 20)
	if err != nil || len(got) != 1 || got[0].TopTool != "mcp.list_files" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestT19_UnreadPathsEmptyInput(t *testing.T) {
	d := setupDB(t)
	got, err := NewRepository(d).UnreadPaths(ctx, win, P, nil, 51)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %v err=%v", got, err)
	}
}
