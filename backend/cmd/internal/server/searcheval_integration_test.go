//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/createadmin"
	"grimoire/lib/util/rankeval"
)

const (
	seFixtureDir   = "../../../testdata/searcheval"
	seEnvTolerance = "SEARCH_EVAL_TOLERANCE"
	seEnvUpdate    = "SEARCH_EVAL_UPDATE_BASELINE"
)

// seSetupMCP seperti setupMCP tetapi dengan MCP_RATE_PER_MINUTE tinggi.
func seSetupMCP(t *testing.T) *mcpEnv {
	t.Helper()
	dbURL, schema, admin := testDBURL(t)
	cfg := testCfg(t, dbURL, map[string]string{
		"RUN_MIGRATION": "true", "COOKIE_SECURE": "false", "MCP_RATE_PER_MINUTE": "10000",
	})
	depsCh := make(chan app.Deps, 1)
	r := startRun(t, cfg, WithAppHook(func(d app.Deps) { depsCh <- d }))
	d := <-depsCh

	const email, pw = "admin@tim.id", "passw0rd-long-enough"
	if _, err := bus.CallTyped[createadmin.Response](context.Background(), d.Bus, constants.BUS_CORE_AUTH_CREATE_ADMIN,
		createadmin.Request{Email: email, Name: "Admin", Password: pw}); err != nil {
		t.Fatal(err)
	}
	e := &mcpEnv{t: t, base: r.base, admin: admin, schema: schema, client: &http.Client{Jar: newJar()}}
	if resp, b := e.rest(http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": pw}); resp.StatusCode != 200 {
		t.Fatalf("login: %d %s", resp.StatusCode, b)
	}
	resp, b := e.rest(http.MethodPost, "/api/projects", map[string]string{"name": "Payments"})
	if resp.StatusCode != 201 {
		t.Fatalf("create project: %d %s", resp.StatusCode, b)
	}
	var pr struct{ ID string }
	_ = json.Unmarshal(b, &pr)
	e.projID, e.project = pr.ID, "Payments"
	e.newKey("search-eval")
	return e
}

func seTolerance(t *testing.T) float64 {
	t.Helper()
	raw := os.Getenv(seEnvTolerance)
	if strings.TrimSpace(raw) == "" {
		return 0.02
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
		t.Fatalf("%s=%q tidak valid (angka 0..1)", seEnvTolerance, raw)
	}
	return v
}

func seSeedCorpus(t *testing.T, e *mcpEnv) {
	t.Helper()
	root := filepath.Join(seFixtureDir, "corpus")
	n := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		e.seedKnowledge(e.projID, filepath.ToSlash(rel), string(b))
		n++
		return nil
	})
	if err != nil {
		t.Fatalf("seed korpus: %v", err)
	}
	if n == 0 {
		t.Fatal("korpus kosong")
	}
}

// seSeedSynonyms mengirim kamus fixture lewat REST; gagal sekali saja menghentikan tes.
func seSeedSynonyms(t *testing.T, e *mcpEnv) {
	t.Helper()
	groups, err := rankeval.LoadSynonyms(filepath.Join(seFixtureDir, "synonyms.yaml"))
	if err != nil {
		t.Fatalf("synonyms.yaml tidak terbaca: %v", err)
	}
	if err := rankeval.ValidateSynonyms(groups); err != nil {
		t.Fatalf("synonyms.yaml tidak valid: %v", err)
	}
	for _, g := range groups {
		resp, b := e.rest(http.MethodPost, "/api/projects/"+e.projID+"/search/synonyms", map[string]any{"terms": g})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("seed sinonim %v: %d %s", g, resp.StatusCode, b)
		}
	}
}

func seSearch(t *testing.T, e *mcpEnv, q string) []string {
	t.Helper()
	r := e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": q, "limit": 10})
	if r.IsError {
		t.Fatalf("search_knowledge error untuk %q: %s", q, r.Raw)
	}
	items, ok := r.StructuredContent["items"].([]any)
	if !ok {
		t.Fatalf("search_knowledge %q: items bukan array: %s", q, r.Raw)
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			t.Fatalf("search_knowledge %q: item tak terduga: %s", q, r.Raw)
		}
		p, ok := m["path"].(string)
		if !ok {
			t.Fatalf("search_knowledge %q: path bukan string: %s", q, r.Raw)
		}
		out = append(out, p)
	}
	return out
}

func TestSearchEval(t *testing.T) {
	g, err := rankeval.Load(filepath.Join(seFixtureDir, "golden.yaml"))
	if err != nil {
		t.Fatalf("golden.yaml tidak terbaca: %v", err)
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("golden.yaml tidak valid: %v", err)
	}
	tol := seTolerance(t)
	update := os.Getenv(seEnvUpdate) == "1"
	basePath := filepath.Join(seFixtureDir, "baseline.json")

	var base rankeval.Baseline
	if !update {
		base, err = rankeval.LoadBaseline(basePath)
		if err != nil {
			t.Fatalf("baseline.json tidak terbaca: %v (buat dengan: make search-eval-baseline)", err)
		}
	}

	e := seSetupMCP(t)
	seSeedCorpus(t, e)
	seSeedSynonyms(t, e)

	results := make(map[string][]string, len(g.Pairs))
	for _, p := range g.Pairs {
		results[p.Query] = seSearch(t, e, p.Query)
	}
	cur := g.Evaluate(results)

	t.Log(rankeval.FormatSummary(cur))
	for _, l := range rankeval.FormatPerKind(cur) {
		t.Log(l)
	}
	for _, f := range g.Failures(results) {
		t.Logf("GAGAL [%s] %q expect=%v rank=%d top5=%v", f.Pair.Kind, f.Pair.Query, f.Pair.Expect, f.Rank, f.Got)
	}

	if update {
		if err := rankeval.SaveBaseline(basePath, cur); err != nil {
			t.Fatal(err)
		}
		t.Logf("baseline.json diperbarui: %s", rankeval.FormatSummary(cur))
		return
	}
	if base.Pairs != cur.Pairs {
		t.Fatalf("jumlah pasangan golden (%d) != baseline (%d); golden berubah, perbarui baseline: make search-eval-baseline", cur.Pairs, base.Pairs)
	}
	for _, d := range rankeval.FormatDrops(rankeval.Compare(cur, base, tol)) {
		t.Errorf("%s, toleransi %.4f", d, tol)
	}
	t.Logf("toleransi=%.4f baseline: %s", tol, rankeval.FormatSummary(base))
}
