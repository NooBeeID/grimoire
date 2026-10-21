//go:build integration

package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

const (
	sqrK1 = "11111111-1111-4111-8111-111111111111"
	sqrK2 = "22222222-2222-4222-8222-222222222222"
)

type sqrEnv struct {
	t      *testing.T
	dbURL  string
	schema string
	admin  *sql.DB
	deps   app.Deps
}

func sqrSetup(t *testing.T) *sqrEnv {
	t.Helper()
	dbURL, schema, admin := testDBURL(t)
	cfg := testCfg(t, dbURL, map[string]string{"RUN_MIGRATION": "true"})
	depsCh := make(chan app.Deps, 1)
	startRun(t, cfg, WithAppHook(func(d app.Deps) { depsCh <- d }))
	return &sqrEnv{t: t, dbURL: dbURL, schema: schema, admin: admin, deps: <-depsCh}
}

type sqrRow struct {
	project, actor, action, target, status string
	at                                     time.Time
	args, result                           map[string]any
}

func (e *sqrEnv) insert(r sqrRow) {
	e.t.Helper()
	if r.status == "" {
		r.status = "ok"
	}
	var actor any
	if r.actor != "" {
		actor = r.actor
	}
	var target any
	if r.target != "" {
		target = r.target
	}
	args, _ := json.Marshal(nonNil(r.args))
	res, _ := json.Marshal(nonNil(r.result))
	_, err := e.admin.Exec("INSERT INTO "+e.schema+".audit_log (occurred_at, actor_type, actor_id, actor_label, project_id, action, target, status, args, result) "+
		"VALUES ($1,'api_key',$2,'k',$3,$4,$5,$6,$7::jsonb,$8::jsonb)",
		r.at, actor, r.project, r.action, target, r.status, string(args), string(res))
	if err != nil {
		e.t.Fatal(err)
	}
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (e *sqrEnv) search(project, actor string, at time.Time, q string, paths []string) {
	e.t.Helper()
	e.insert(sqrRow{project: project, actor: actor, at: at, action: "mcp.search_knowledge",
		args:   map[string]any{"query": q},
		result: map[string]any{"count": len(paths), "total": len(paths), "result_paths": paths}})
}

func (e *sqrEnv) read(project, actor string, at time.Time, path string) {
	e.t.Helper()
	e.insert(sqrRow{project: project, actor: actor, at: at, action: "mcp.read_file", target: path})
}

func (e *sqrEnv) report(req searchquality.Request) (searchquality.Report, error) {
	e.t.Helper()
	res, err := bus.CallTyped[searchquality.Response](context.Background(), e.deps.Bus,
		constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, req)
	return res.Report, err
}

// window mengembalikan request untuk project dengan rentang yang mencakup t0 (2 jam lalu).
func (e *sqrEnv) window(project string, t0 time.Time) searchquality.Request {
	return searchquality.Request{ProjectID: project, Since: t0.Add(-time.Hour), Until: t0.Add(time.Hour)}
}

func sqrT0() time.Time { return time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second) }

func sqrMustReport(t *testing.T, e *sqrEnv, req searchquality.Request) searchquality.Report {
	t.Helper()
	r, err := e.report(req)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func sqrBuckets(r searchquality.Report) map[string]int {
	m := map[string]int{}
	for _, b := range r.RankOpened {
		m[b.Label] = b.Count
	}
	return m
}

func sqrAllZero(t *testing.T, r searchquality.Report) {
	t.Helper()
	if len(r.RankOpened) != 11 {
		t.Fatalf("bucket = %d", len(r.RankOpened))
	}
	for _, b := range r.RankOpened {
		if b.Count != 0 {
			t.Fatalf("bucket %s = %d, mau 0", b.Label, b.Count)
		}
	}
}

func TestT11_AC7_FollowUpRank(t *testing.T) {
	e := sqrSetup(t)
	t0 := sqrT0()
	paths := []string{"a.md", "b.md", "c.md"}

	p := uuid.NewString()
	e.search(p, sqrK1, t0, "q", paths)
	e.read(p, sqrK1, t0.Add(30*time.Second), "b.md")
	r := sqrMustReport(t, e, e.window(p, t0))
	if r.FollowUp.Count != 1 || r.FollowUp.Denominator != 1 || r.FollowUp.Rate != 1 {
		t.Fatalf("follow-up: %+v", r.FollowUp)
	}
	b := sqrBuckets(r)
	if b["2"] != 1 {
		t.Fatalf("bucket: %+v", b)
	}
	for k, v := range b {
		if k != "2" && v != 0 {
			t.Fatalf("bucket %s = %d", k, v)
		}
	}

	// negatif (a): aktor lain
	pa := uuid.NewString()
	e.search(pa, sqrK1, t0, "q", paths)
	e.read(pa, sqrK2, t0.Add(30*time.Second), "b.md")
	r = sqrMustReport(t, e, e.window(pa, t0))
	if r.FollowUp.Count != 0 || r.FollowUp.Denominator != 1 {
		t.Fatalf("aktor lain: %+v", r.FollowUp)
	}
	sqrAllZero(t, r)

	// negatif (b): read pada project lain
	pb, other := uuid.NewString(), uuid.NewString()
	e.search(pb, sqrK1, t0, "q", paths)
	e.read(other, sqrK1, t0.Add(30*time.Second), "b.md")
	r = sqrMustReport(t, e, e.window(pb, t0))
	if r.FollowUp.Count != 0 {
		t.Fatalf("project lain: %+v", r.FollowUp)
	}
	sqrAllZero(t, r)

	// negatif (c): di luar jendela
	pc := uuid.NewString()
	e.search(pc, sqrK1, t0, "q", paths)
	e.read(pc, sqrK1, t0.Add(3*time.Minute), "b.md")
	r = sqrMustReport(t, e, e.window(pc, t0))
	if r.FollowUp.Count != 0 {
		t.Fatalf("di luar jendela: %+v", r.FollowUp)
	}
	sqrAllZero(t, r)

	// tepi: tepat 120s dihitung
	pd := uuid.NewString()
	e.search(pd, sqrK1, t0, "q", paths)
	e.read(pd, sqrK1, t0.Add(120*time.Second), "a.md")
	r = sqrMustReport(t, e, e.window(pd, t0))
	if r.FollowUp.Count != 1 || sqrBuckets(r)["1"] != 1 {
		t.Fatalf("tepi 120s: %+v %+v", r.FollowUp, sqrBuckets(r))
	}
}

func TestT11_AC8_Reformulation(t *testing.T) {
	e := sqrSetup(t)
	t0 := sqrT0()

	p := uuid.NewString()
	e.search(p, sqrK1, t0, "login", []string{"a.md"})
	e.search(p, sqrK1, t0.Add(60*time.Second), "oauth login", []string{"b.md"})
	r := sqrMustReport(t, e, e.window(p, t0))
	if r.Searches != 2 || r.Reformulation.Count != 1 || r.Reformulation.Denominator != 2 {
		t.Fatalf("%+v", r)
	}

	p2 := uuid.NewString()
	e.search(p2, sqrK1, t0, "login", []string{"a.md"})
	e.search(p2, sqrK1, t0.Add(60*time.Second), "login", []string{"a.md"})
	if r := sqrMustReport(t, e, e.window(p2, t0)); r.Reformulation.Count != 0 {
		t.Fatalf("query sama: %+v", r.Reformulation)
	}

	p3 := uuid.NewString()
	e.search(p3, sqrK1, t0, "login", []string{"a.md"})
	e.search(p3, sqrK1, t0.Add(60*time.Second), "Login  ", []string{"a.md"})
	if r := sqrMustReport(t, e, e.window(p3, t0)); r.Reformulation.Count != 0 {
		t.Fatalf("beda kapital/spasi: %+v", r.Reformulation)
	}

	p4 := uuid.NewString()
	e.search(p4, sqrK1, t0, "login", []string{"a.md"})
	e.search(p4, sqrK1, t0.Add(130*time.Second), "oauth login", []string{"b.md"})
	if r := sqrMustReport(t, e, e.window(p4, t0)); r.Reformulation.Count != 0 {
		t.Fatalf("di luar jendela: %+v", r.Reformulation)
	}
}

func TestT11_AC9_LegacyEntries(t *testing.T) {
	e := sqrSetup(t)
	t0 := sqrT0()
	p := uuid.NewString()
	for i, n := range []int{0, 2} {
		e.insert(sqrRow{project: p, actor: sqrK1, at: t0.Add(time.Duration(i) * time.Minute), action: "mcp.search_knowledge",
			args: map[string]any{"query": "q" + string(rune('a'+i))}, result: map[string]any{"count": n, "total": n}})
	}
	e.read(p, sqrK1, t0.Add(time.Minute+20*time.Second), "a.md")
	r := sqrMustReport(t, e, e.window(p, t0))
	if r.ZeroResult.Count != 1 || r.ZeroResult.Denominator != 2 || r.LegacySearches != 2 ||
		r.FollowUp.Denominator != 0 || r.FollowUp.Count != 0 {
		t.Fatalf("%+v", r)
	}
	sqrAllZero(t, r)
}

func TestT11_PagingAcrossPages(t *testing.T) {
	e := sqrSetup(t)
	t0 := sqrT0()
	p := uuid.NewString()
	// 2.300 read oleh aktor lain; occurred_at identik pada kelompok 10 baris.
	if _, err := e.admin.Exec("INSERT INTO "+e.schema+".audit_log (occurred_at, actor_type, actor_id, actor_label, project_id, action, target, status) "+
		"SELECT $1::timestamptz + ((g/10) * interval '1 second'), 'api_key', $2::uuid, 'k', $3::uuid, 'mcp.read_file', 'z.md', 'ok' FROM generate_series(1,2300) g",
		t0.Add(-30*time.Minute), sqrK2, p); err != nil {
		t.Fatal(err)
	}
	paths := []string{"a.md", "b.md"}
	e.search(p, sqrK1, t0, "q1", paths)
	e.read(p, sqrK1, t0.Add(10*time.Second), "a.md")
	e.search(p, sqrK1, t0.Add(30*time.Minute), "q2", paths)
	e.search(p, sqrK1, t0.Add(31*time.Minute), "q3", paths)
	// q3 +65s (dalam jendela 120s), q2 +125s (di luar jendela).
	e.read(p, sqrK1, t0.Add(32*time.Minute+5*time.Second), "b.md")
	r := sqrMustReport(t, e, e.window(p, t0))
	if r.Searches != 3 || r.FollowUp.Count != 2 || r.FollowUp.Denominator != 3 {
		t.Fatalf("%+v", r)
	}
	b := sqrBuckets(r)
	if b["1"] != 1 || b["2"] != 1 {
		t.Fatalf("bucket: %+v", b)
	}
}

func TestT11_RangeBoundaries(t *testing.T) {
	e := sqrSetup(t)
	t0 := sqrT0()
	since, until := t0, t0.Add(time.Hour)
	p := uuid.NewString()
	paths := []string{"a.md"}
	e.search(p, sqrK1, since.Add(-time.Second), "lama", paths)
	e.search(p, sqrK2, since, "tepat since", paths)
	e.search(p, sqrK1, until.Add(-10*time.Second), "hampir until", paths)
	e.read(p, sqrK1, until.Add(30*time.Second), "a.md")
	e.search(p, sqrK2, until, "tepat until", paths)
	r := sqrMustReport(t, e, searchquality.Request{ProjectID: p, Since: since, Until: until})
	if r.Searches != 2 {
		t.Fatalf("searches = %d, mau 2", r.Searches)
	}
	if r.FollowUp.Count != 1 || r.FollowUp.Denominator != 2 {
		t.Fatalf("follow-up: %+v", r.FollowUp)
	}
}

func TestT11_OnlyOkAndOnlyProject(t *testing.T) {
	e := sqrSetup(t)
	t0 := sqrT0()
	p, other := uuid.NewString(), uuid.NewString()
	paths := []string{"a.md"}
	e.search(p, sqrK1, t0, "ok", paths)
	for _, st := range []string{"error", "denied"} {
		e.insert(sqrRow{project: p, actor: sqrK1, at: t0.Add(time.Second), action: "mcp.search_knowledge", status: st,
			args: map[string]any{"query": "x"}, result: map[string]any{"total": 1, "count": 1, "result_paths": paths}})
	}
	e.search(p, "", t0.Add(2*time.Second), "anon", paths)
	e.search(other, sqrK1, t0.Add(3*time.Second), "lain", paths)
	r := sqrMustReport(t, e, e.window(p, t0))
	if r.Searches != 1 || r.Reformulation.Count != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestT11_ErrorsViaBus(t *testing.T) {
	e := sqrSetup(t)
	now := time.Now()
	good := searchquality.Request{ProjectID: uuid.NewString(), Since: now.Add(-time.Hour)}
	cases := map[string]searchquality.Request{
		"project": {ProjectID: "bukan-uuid", Since: good.Since},
		"since":   {ProjectID: good.ProjectID},
		"window":  {ProjectID: good.ProjectID, Since: good.Since, Window: 2 * time.Hour},
		"until":   {ProjectID: good.ProjectID, Since: good.Since, Until: good.Since},
	}
	for name, req := range cases {
		if _, err := e.report(req); !errors.Is(err, errs.ErrInvalid) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	_, err := e.deps.Bus.Call(context.Background(), constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, &searchquality.Request{})
	if !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}
}

func TestT11_UnknownProject(t *testing.T) {
	e := sqrSetup(t)
	r := sqrMustReport(t, e, searchquality.Request{ProjectID: uuid.NewString(), Since: time.Now().Add(-time.Hour)})
	if r.Searches != 0 || r.ZeroResult.Rate != 0 || r.FollowUp.Rate != 0 || r.Reformulation.Rate != 0 || r.LegacySearches != 0 {
		t.Fatalf("%+v", r)
	}
	sqrAllZero(t, r)
	for i := 1; i <= 10; i++ {
		if r.RankOpened[i-1].Label != itoaSQR(i) {
			t.Fatalf("label %d = %s", i, r.RankOpened[i-1].Label)
		}
	}
	if r.RankOpened[10].Label != searchquality.NotInResults {
		t.Fatalf("label terakhir = %s", r.RankOpened[10].Label)
	}
}

func itoaSQR(i int) string { return strconv.Itoa(i) }

func sqrBuildCLI(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	backend := filepath.Join(filepath.Dir(file), "..", "..", "..")
	out := filepath.Join(os.TempDir(), "grimoire-sqr-"+uuid.NewString()[:8])
	cmd := exec.Command("go", "build", "-o", out, "./cmd/cli")
	cmd.Dir = backend
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	t.Cleanup(func() { _ = os.Remove(out) })
	return out
}

func sqrRunCLI(t *testing.T, bin, dbURL string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"DATABASE_URL=" + dbURL, "SESSION_SECRET=" + testSecret, "HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH")}
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return o.String(), e.String(), code
}

func TestT11_AC10_CLI(t *testing.T) {
	e := sqrSetup(t)
	bin := sqrBuildCLI(t)
	t0 := sqrT0()
	p := uuid.NewString()
	e.search(p, sqrK1, t0, "login", []string{"a.md", "b.md", "c.md"})
	e.read(p, sqrK1, t0.Add(30*time.Second), "b.md")
	e.search(p, sqrK1, t0.Add(90*time.Second), "oauth login", []string{"d.md"})

	out, errOut, code := sqrRunCLI(t, bin, e.dbURL, "search-quality", "report", "--project", p, "--since", "7d")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{"Zero-result", "Follow-up", "Reformulasi", "Rank dibuka", "Entri lama dilewati"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout tidak memuat %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "1/2") {
		t.Errorf("follow-up 1/2 tidak ada:\n%s", out)
	}

	out, errOut, code = sqrRunCLI(t, bin, e.dbURL, "search-quality", "report", "--project", p, "--since", "7d", "--json")
	if code != 0 || !json.Valid([]byte(out)) {
		t.Fatalf("code=%d valid=%v stderr=%s out=%s", code, json.Valid([]byte(out)), errOut, out)
	}
	var res searchquality.Response
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.Report.ProjectID != p || res.Report.FollowUp.Count != 1 || len(res.Report.RankOpened) != 11 || res.Report.RankOpened[1].Count != 1 {
		t.Fatalf("%+v", res.Report)
	}

	out, errOut, code = sqrRunCLI(t, bin, e.dbURL, "search-quality", "report", "--since", "7d")
	if code != 2 || out != "" || !strings.Contains(errOut, "Pemakaian:") {
		t.Fatalf("tanpa project: code=%d out=%q err=%q", code, out, errOut)
	}
	out, errOut, code = sqrRunCLI(t, bin, e.dbURL, "search-quality", "report", "--project", "bukan-uuid", "--since", "7d")
	if code != 1 || out != "" || !strings.Contains(errOut, "search-quality: project_id tidak valid") {
		t.Fatalf("bukan uuid: code=%d out=%q err=%q", code, out, errOut)
	}
	_, _, code = sqrRunCLI(t, bin, e.dbURL, "search-quality", "report", "--project", p, "--since", "7d", "--window", "2h")
	if code != 2 {
		t.Fatalf("window 2h: code=%d", code)
	}
}
