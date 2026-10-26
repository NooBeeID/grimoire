//go:build integration

package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Tes integrasi T18: get_file_outline, read_section, list_recent_changes lewat /mcp.

const (
	olGuide = "# Intro\nhalo 日本\n```sh\n# bukan\n```\n## Setup\nlangkah\n# Intro\n## Setup\nkedua\n"
	olNotes = "# bukan heading karena bukan markdown\n"
)

var olTools = []string{"get_file_outline", "read_section", "list_recent_changes"}

// olEnv: project A (docs/guide.md, notes.txt, spec petstore) dan project B (secret/b.md).
func setupOutline(t *testing.T) *ktEnv {
	t.Helper()
	e := setupMCP(t)
	k := &ktEnv{mcpEnv: e}
	e.seedKnowledge(e.projID, "docs/guide.md", olGuide)
	e.seedKnowledge(e.projID, "notes.txt", olNotes)
	e.asUpload(e.projID, map[string]string{"petstore.yaml": asPetstoreYAML})
	k.projB = e.newProject("Other")
	e.seedKnowledge(k.projB, "secret/b.md", "# Rahasia\nSECRET-B\n")
	return k
}

func TestT18_AC3_ToolsListed(t *testing.T) {
	k := setupOutline(t)
	if resp, b := k.mcp(k.rawKey, initBody, nil); resp.StatusCode != 200 {
		t.Fatalf("initialize: %d %s", resp.StatusCode, b)
	}
	_, body := k.mcp(k.rawKey, listBody, nil)
	var out struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Annotations struct {
					ReadOnlyHint bool `json:"readOnlyHint"`
				} `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("bukan JSON: %v: %s", err, body)
	}
	got := map[string]bool{}
	for _, tl := range out.Result.Tools {
		got[tl.Name] = tl.Annotations.ReadOnlyHint
	}
	for _, name := range olTools {
		if ro, ok := got[name]; !ok || !ro {
			t.Fatalf("tool %s ada=%v readOnlyHint=%v: %s", name, ok, ro, body)
		}
	}
}

func TestT18_Int_Outline(t *testing.T) {
	k := setupOutline(t)
	res := k.mcpCall(k.rawKey, "get_file_outline", map[string]any{"path": "docs/guide.md"})
	if res.IsError {
		t.Fatalf("isError: %s", res.Text)
	}
	if s, _ := res.StructuredContent["updated_at"].(string); s == "" {
		t.Fatalf("updated_at kosong: %s", res.Raw)
	}
	hs, _ := res.StructuredContent["headings"].([]any)
	if len(hs) != 4 {
		t.Fatalf("heading = %d, want 4 (heading di code fence tidak dihitung): %s", len(hs), res.Raw)
	}
	if strings.Contains(res.Text, "bukan") {
		t.Fatalf("heading dalam fence bocor: %s", res.Text)
	}

	res = k.mcpCall(k.rawKey, "get_file_outline", map[string]any{"path": "notes.txt"})
	if res.IsError {
		t.Fatalf("isError: %s", res.Text)
	}
	if hs, _ := res.StructuredContent["headings"].([]any); len(hs) != 0 {
		t.Fatalf("non-Markdown harus kosong: %s", res.Raw)
	}
	if n, _ := res.StructuredContent["size_bytes"].(float64); n <= 0 {
		t.Fatalf("size_bytes: %s", res.Raw)
	}
	if n := k.countAudit("mcp.get_file_outline"); n != 2 {
		t.Fatalf("audit = %d, want 2", n)
	}
}

func TestT18_AC2_ReadSectionExact(t *testing.T) {
	k := setupOutline(t)
	res := k.mcpCall(k.rawKey, "read_section", map[string]any{"path": "docs/guide.md", "heading": "Setup", "occurrence": 2})
	if res.IsError || res.Text != "## Setup\nkedua\n" {
		t.Fatalf("res=%+v", res)
	}
	res = k.mcpCall(k.rawKey, "read_section", map[string]any{"path": "docs/guide.md", "heading": "Intro"})
	if res.IsError || strings.Contains(res.Text, "kedua") || !strings.HasPrefix(res.Text, "# Intro\nhalo 日本\n") {
		t.Fatalf("res=%+v", res)
	}
	res = k.mcpCall(k.rawKey, "read_section", map[string]any{"path": "docs/guide.md", "heading": "Nope"})
	if !res.IsError || !strings.Contains(res.Text, "get_file_outline") {
		t.Fatalf("res=%+v", res)
	}
	// limit 17 memotong di dalam rune kedua ("本"): hasil harus UTF-8 valid dan berhenti sebelum rune itu.
	res = k.mcpCall(k.rawKey, "read_section", map[string]any{"path": "docs/guide.md", "heading": "Intro", "limit": 17})
	if res.IsError || !utf8.ValidString(res.Text) || !strings.HasPrefix(res.Text, "# Intro\nhalo 日\n\n[bagian terpotong:") {
		t.Fatalf("res=%+v", res)
	}
	if n := k.countAudit("mcp.read_section"); n != 4 {
		t.Fatalf("audit = %d, want 4", n)
	}
}

func TestT18_Int_RecentChanges(t *testing.T) {
	k := setupOutline(t)
	res := k.mcpCall(k.rawKey, "list_recent_changes", map[string]any{"since": "1h"})
	if res.IsError {
		t.Fatalf("isError: %s", res.Text)
	}
	items, _ := res.StructuredContent["items"].([]any)
	var guide, spec map[string]any
	for _, it := range items {
		m := it.(map[string]any)
		switch {
		case m["path"] == "docs/guide.md":
			guide = m
		case m["name"] == "petstore":
			spec = m
		}
		if m["path"] == "secret/b.md" {
			t.Fatalf("file project lain bocor: %s", res.Raw)
		}
	}
	if guide == nil || guide["created"] != true || spec == nil {
		t.Fatalf("guide=%v spec=%v raw=%s", guide, spec, res.Raw)
	}
	if _, has := guide["updated_by"]; has || strings.Contains(res.Raw, "updated_by") {
		t.Fatalf("identitas pengubah bocor: %s", res.Raw)
	}

	// Ubah file lewat REST PUT: created harus false.
	path := "/api/projects/" + k.projID + "/knowledge/files?path=docs/guide.md"
	resp, b := k.rest(http.MethodGet, path, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get file: %d %s", resp.StatusCode, b)
	}
	var f struct{ Version string }
	_ = json.Unmarshal(b, &f)
	resp, b = k.rest(http.MethodPut, path, map[string]string{"content": olGuide + "tambahan\n", "version": f.Version})
	if resp.StatusCode != 200 {
		t.Fatalf("put file: %d %s", resp.StatusCode, b)
	}
	res = k.mcpCall(k.rawKey, "list_recent_changes", map[string]any{"since": "1h", "kind": "files"})
	items, _ = res.StructuredContent["items"].([]any)
	found := false
	for _, it := range items {
		if m := it.(map[string]any); m["path"] == "docs/guide.md" {
			found = true
			if m["created"] != false {
				t.Fatalf("setelah update created harus false: %v", m)
			}
		}
	}
	if !found {
		t.Fatalf("guide tidak ada: %s", res.Raw)
	}

	res = k.mcpCall(k.rawKey, "list_recent_changes", map[string]any{"since": "1h", "kind": "specs", "path": "docs"})
	if !res.IsError {
		t.Fatalf("kind specs + path harus isError: %s", res.Text)
	}

	var target string
	q := "SELECT COALESCE(target, '') FROM " + k.schema + ".audit_log WHERE action = 'mcp.list_recent_changes' LIMIT 1"
	if err := k.admin.QueryRow(q).Scan(&target); err != nil {
		t.Fatal(err)
	}
	if target != "" {
		t.Fatalf("target audit = %q, want kosong", target)
	}
}

func TestT18_AntiGoal_ProjectArgRejected(t *testing.T) {
	k := setupOutline(t)
	calls := map[string]map[string]any{
		"get_file_outline":    {"path": "secret/b.md"},
		"read_section":        {"path": "secret/b.md", "heading": "Rahasia"},
		"list_recent_changes": {"since": "1h"},
	}
	for name, args := range calls {
		for _, alias := range []string{"project_id", "projectId", "project"} {
			a := map[string]any{alias: k.projB}
			for kk, v := range args {
				a[kk] = v
			}
			res := k.mcpCall(k.rawKey, name, a)
			if !res.IsError || !strings.Contains(res.Text, "ditentukan oleh API key") || strings.Contains(res.Text, "SECRET-B") {
				t.Fatalf("%s/%s: %+v", name, alias, res)
			}
		}
	}
	res := k.mcpCall(k.rawKey, "read_section", map[string]any{"path": "secret/b.md", "heading": "Rahasia"})
	if !res.IsError || strings.Contains(res.Text, "SECRET-B") {
		t.Fatalf("isolasi project: %+v", res)
	}
}

func TestT18_AC3_DashboardAggregates(t *testing.T) {
	k := setupOutline(t)
	k.mcpCall(k.rawKey, "get_file_outline", map[string]any{"path": "docs/guide.md"})
	k.mcpCall(k.rawKey, "read_section", map[string]any{"path": "docs/guide.md", "heading": "Intro"})
	k.mcpCall(k.rawKey, "list_recent_changes", map[string]any{"since": "1h"})
	time.Sleep(1100 * time.Millisecond) // batas atas jendela summary dibulatkan ke detik (eksklusif)
	resp, b := k.rest(http.MethodGet, "/api/audit-log/summary?range=24h", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("summary: %d %s", resp.StatusCode, b)
	}
	var out struct {
		ToolUsage []struct {
			Tool  string
			Calls int
		} `json:"tool_usage"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, u := range out.ToolUsage {
		got[u.Tool] = u.Calls
	}
	for _, name := range olTools {
		if got[name] != 1 {
			t.Fatalf("tool_usage[%s] = %d, want 1: %s", name, got[name], b)
		}
	}
}

func TestT18_Int_AuditHasNoContent(t *testing.T) {
	k := setupMCP(t)
	k.seedKnowledge(k.projID, "docs/s.md", "# Judul\nSECRET-OUTLINE-42\n## SECRET-OUTLINE-42 heading\n")
	k.mcpCall(k.rawKey, "get_file_outline", map[string]any{"path": "docs/s.md"})
	k.mcpCall(k.rawKey, "read_section", map[string]any{"path": "docs/s.md", "heading": "Judul"})
	k.mcpCall(k.rawKey, "list_recent_changes", map[string]any{"since": "1h"})
	var n int
	q := "SELECT count(*) FROM " + k.schema + ".audit_log WHERE action LIKE 'mcp.%' AND (result::text LIKE '%SECRET-OUTLINE-42%' OR COALESCE(args::text,'') LIKE '%SECRET-OUTLINE-42%')"
	if err := k.admin.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d baris audit memuat isi file", n)
	}
	for _, name := range olTools {
		if k.countAudit("mcp."+name) != 1 {
			t.Fatalf("audit %s != 1", name)
		}
	}
}
