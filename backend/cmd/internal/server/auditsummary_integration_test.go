//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/createadmin"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

const (
	sumAdminEmail = "admin@tim.id"
	sumAdminPass  = "passw0rd-long-enough"
	sumDevEmail   = "dev@tim.id"
	sumDevPass    = "dev-passw0rd-long"
)

type sumEnv struct {
	*sqrEnv
	base  string
	admin *http.Client
}

func sumLogin(t *testing.T, base, email, pw string) *http.Client {
	t.Helper()
	c := &http.Client{Jar: newJar()}
	resp, b := sumDo(t, c, base, http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": pw})
	if resp.StatusCode != 200 {
		t.Fatalf("login %s: %d %s", email, resp.StatusCode, b)
	}
	return c
}

func sumDo(t *testing.T, c *http.Client, base, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "grimoire")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func sumSetup(t *testing.T) *sumEnv {
	t.Helper()
	dbURL, schema, admin := testDBURL(t)
	cfg := testCfg(t, dbURL, map[string]string{"RUN_MIGRATION": "true"})
	depsCh := make(chan app.Deps, 1)
	r := startRun(t, cfg, WithAppHook(func(d app.Deps) { depsCh <- d }))
	d := <-depsCh
	if _, err := bus.CallTyped[createadmin.Response](context.Background(), d.Bus, constants.BUS_CORE_AUTH_CREATE_ADMIN,
		createadmin.Request{Email: sumAdminEmail, Name: "Admin", Password: sumAdminPass}); err != nil {
		t.Fatal(err)
	}
	e := &sumEnv{
		sqrEnv: &sqrEnv{t: t, dbURL: dbURL, schema: schema, admin: admin, deps: d},
		base:   r.base,
		admin:  sumLogin(t, r.base, sumAdminEmail, sumAdminPass),
	}
	return e
}

func (e *sumEnv) summary(c *http.Client, query string) (int, map[string]any) {
	e.t.Helper()
	resp, b := sumDo(e.t, c, e.base, http.MethodGet, "/api/audit-log/summary"+query, nil)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return resp.StatusCode, m
}

func num(v any) int { f, _ := v.(float64); return int(f) }

func TestT15_AC8_EqualsCLI(t *testing.T) {
	e := sumSetup(t)
	bin := sqrBuildCLI(t)
	t0 := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	p := uuid.NewString()
	paths := []string{"a.md", "b.md", "c.md"}
	e.search(p, sqrK1, t0, "login", paths)
	e.read(p, sqrK1, t0.Add(30*time.Second), "b.md")
	e.search(p, sqrK1, t0.Add(90*time.Second), "oauth login", []string{"d.md"})
	e.search(p, sqrK2, t0.Add(10*time.Minute), "zero", nil)
	e.insert(sqrRow{project: p, actor: sqrK1, at: t0.Add(20 * time.Minute), action: "mcp.search_knowledge",
		args: map[string]any{"query": "legacy"}, result: map[string]any{"count": 1, "total": 1}})

	code, m := e.summary(e.admin, "?range=24h&project_id="+p)
	if code != 200 {
		t.Fatalf("status %d: %v", code, m)
	}
	sq, _ := m["search_quality"].(map[string]any)
	if sq == nil || sq["available"] != true {
		t.Fatalf("search_quality: %v", m["search_quality"])
	}
	from, to := m["from"].(string), m["to"].(string)

	out, errOut, ec := sqrRunCLI(t, bin, e.dbURL, "search-quality", "report", "--project", p, "--since", from, "--until", to, "--json")
	if ec != 0 {
		t.Fatalf("cli code=%d %s", ec, errOut)
	}
	var res searchquality.Response
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	rep := res.Report

	if num(sq["searches"]) != rep.Searches || num(sq["window_seconds"]) != rep.WindowSeconds || num(sq["legacy_searches"]) != rep.LegacySearches {
		t.Fatalf("api=%v cli=%+v", sq, rep)
	}
	for name, want := range map[string]searchquality.Metric{"zero_result": rep.ZeroResult, "follow_up": rep.FollowUp, "reformulation": rep.Reformulation} {
		got, _ := sq[name].(map[string]any)
		if num(got["count"]) != want.Count || num(got["denominator"]) != want.Denominator || got["rate"].(float64) != want.Rate {
			t.Fatalf("%s: api=%v cli=%+v", name, got, want)
		}
	}
	ro, _ := sq["rank_opened"].([]any)
	if len(ro) != 11 || len(rep.RankOpened) != 11 {
		t.Fatalf("rank_opened %d/%d", len(ro), len(rep.RankOpened))
	}
	for i, b := range rep.RankOpened {
		g := ro[i].(map[string]any)
		if g["label"] != b.Label || num(g["count"]) != b.Count {
			t.Fatalf("bucket %d: %v vs %+v", i, g, b)
		}
	}
	if num(sq["window_seconds"]) != 120 {
		t.Fatalf("window_seconds %v", sq["window_seconds"])
	}
}

func TestT15_AC2_AC3_EndToEnd(t *testing.T) {
	e := sumSetup(t)
	now := time.Now().UTC()
	at := now.Add(-time.Hour)
	A, B := uuid.NewString(), uuid.NewString()
	for i := 0; i < 3; i++ {
		e.insert(sqrRow{project: A, actor: sqrK1, at: at, action: "mcp.read_file", target: "a.md", result: map[string]any{"duration_ms": 10 * (i + 1)}})
	}
	e.insert(sqrRow{project: A, actor: sqrK1, at: at, action: "mcp.read_file", target: "a.md", status: "error"})
	for i := 0; i < 2; i++ {
		e.search(B, sqrK2, at, "q", []string{"x.md"})
	}
	for _, a := range []string{"mcp.initialize", "mcp.tools_list", "mcp.auth.rejected", "mcp.rate_limited", "mcp.unknown_tool"} {
		e.insert(sqrRow{project: A, actor: sqrK1, at: at, action: a, status: "denied"})
	}
	var before int
	_ = e.sqrEnv.admin.QueryRow("SELECT count(*) FROM " + e.schema + ".audit_log").Scan(&before)

	code, m := e.summary(e.admin, "?range=24h")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if num(m["total_calls"]) != 6 || num(m["total_errors"]) != 1 {
		t.Fatalf("totals: %v %v", m["total_calls"], m["total_errors"])
	}
	tp := m["top_projects"].([]any)
	if len(tp) != 2 || tp[0].(map[string]any)["project_id"] != A || num(tp[0].(map[string]any)["calls"]) != 4 || num(tp[1].(map[string]any)["calls"]) != 2 {
		t.Fatalf("top_projects %v", tp)
	}
	tu := m["tool_usage"].([]any)
	if len(tu) != 2 || tu[0].(map[string]any)["tool"] != "read_file" {
		t.Fatalf("tool_usage %v", tu)
	}
	if m["search_quality"] != nil {
		t.Fatalf("search_quality tanpa project harus null: %v", m["search_quality"])
	}
	var after int
	_ = e.sqrEnv.admin.QueryRow("SELECT count(*) FROM " + e.schema + ".audit_log").Scan(&after)
	// login admin menulis audit sebelum 'before' diukur; pemanggilan summary tidak boleh menambah entri.
	if after != before {
		t.Fatalf("audit_log berubah %d -> %d", before, after)
	}
}

func TestT15_AC7_Validation(t *testing.T) {
	e := sumSetup(t)

	for _, tc := range []struct{ q, field string }{
		{"?range=12h", "range"},
		{"?project_id=not-uuid", "project_id"},
	} {
		resp, b := sumDo(t, e.admin, e.base, http.MethodGet, "/api/audit-log/summary"+tc.q, nil)
		if resp.StatusCode != 400 {
			t.Fatalf("%s: %d", tc.q, resp.StatusCode)
		}
		var env struct {
			Error struct {
				Code    string
				Details map[string]any
			}
		}
		_ = json.Unmarshal(b, &env)
		if env.Error.Code != "INVALID_ARGUMENT" || env.Error.Details["field"] != tc.field {
			t.Fatalf("%s: %s", tc.q, b)
		}
	}

	// tanpa sesi
	resp, _ := sumDo(t, &http.Client{}, e.base, http.MethodGet, "/api/audit-log/summary", nil)
	if resp.StatusCode != 401 {
		t.Fatalf("anon: %d", resp.StatusCode)
	}

	// developer
	resp, b := sumDo(t, e.admin, e.base, http.MethodPost, "/api/users", map[string]string{"email": sumDevEmail, "name": "Dev", "role": "developer", "password": sumDevPass})
	if resp.StatusCode != 201 {
		t.Fatalf("create dev: %d %s", resp.StatusCode, b)
	}
	dev := sumLogin(t, e.base, sumDevEmail, sumDevPass)
	newPass := "dev-baru-passw0rd-long"
	if resp, b := sumDo(t, dev, e.base, http.MethodPost, "/api/auth/change-password", map[string]string{"current_password": sumDevPass, "new_password": newPass}); resp.StatusCode >= 300 {
		t.Fatalf("change-password: %d %s", resp.StatusCode, b)
	}
	dev = sumLogin(t, e.base, sumDevEmail, newPass)
	resp, b = sumDo(t, dev, e.base, http.MethodGet, "/api/audit-log/summary", nil)
	if resp.StatusCode != 403 {
		t.Fatalf("developer: %d %s", resp.StatusCode, b)
	}
}
