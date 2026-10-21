//go:build integration

package server

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

func ktItems(t *testing.T, r ktResult) []map[string]any {
	t.Helper()
	raw, ok := r.StructuredContent["items"].([]any)
	if !ok {
		t.Fatalf("structuredContent.items bukan array: %s", r.Raw)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		out = append(out, it.(map[string]any))
	}
	return out
}

func TestT08_Int_AC4_SearchSnippet(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "search_knowledge", map[string]any{"query": "rate limit"})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	if !strings.Contains(r.Text, "docs/guides/b.md") || !strings.Contains(r.Text, "(skor ") || !strings.Contains(r.Text, "**rate**") {
		t.Fatalf("teks: %s", r.Text)
	}
	first := strings.SplitN(r.Text, "\n", 2)[0]
	if !regexp.MustCompile(`^\d+ dari \d+ hasil untuk "rate limit"`).MatchString(first) {
		t.Fatalf("baris pertama: %q", first)
	}
	items := ktItems(t, r)
	if len(items) == 0 || !strings.Contains(items[0]["snippet"].(string), "**rate**") {
		t.Fatalf("items: %v", items)
	}
	if total, _ := r.StructuredContent["total"].(float64); total < 1 {
		t.Fatalf("total: %v", r.StructuredContent["total"])
	}
}

func TestT08_Int_AC4_SpecialChars(t *testing.T) {
	k := setupKnowledgeTools(t)
	for _, q := range []string{`a & b | !c:* (x)`, `'"`, `)(`, `:*`, `\`, `<->`} {
		r := k.mcpCall(k.rawKey, "search_knowledge", map[string]any{"query": q})
		if r.IsError {
			t.Fatalf("q=%q isError: %s", q, r.Raw)
		}
		if r.StructuredContent["query"] != q {
			t.Fatalf("q=%q structured.query=%v", q, r.StructuredContent["query"])
		}
	}
}

func TestT08_Int_AC4_PrefixFilter(t *testing.T) {
	k := setupKnowledgeTools(t)
	k.seedKnowledge(k.projID, "docs/c.md", "rate card harga\n")
	for _, p := range []string{"docs/guides", "/docs/guides/"} {
		r := k.mcpCall(k.rawKey, "search_knowledge", map[string]any{"query": "rate", "path": p})
		if r.IsError {
			t.Fatalf("path=%q: %s", p, r.Raw)
		}
		items := ktItems(t, r)
		if len(items) == 0 {
			t.Fatalf("path=%q tanpa hasil: %s", p, r.Raw)
		}
		for _, it := range items {
			if !strings.HasPrefix(it["path"].(string), "docs/guides/") {
				t.Fatalf("path=%q bocor: %v", p, it["path"])
			}
		}
	}
	r := k.mcpCall(k.rawKey, "search_knowledge", map[string]any{"query": "rate", "path": "other"})
	if r.IsError || !strings.Contains(r.Text, "Tidak ada hasil") || !strings.Contains(r.Text, `di other/`) {
		t.Fatalf("%s", r.Raw)
	}
}

func TestT08_Int_AC5_SearchProjectIsolation(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "search_knowledge", map[string]any{"query": "SECRET-B"})
	if r.IsError || len(ktItems(t, r)) != 0 {
		t.Fatalf("A melihat B: %s", r.Raw)
	}
	if strings.Contains(strings.Replace(r.Text, `"SECRET-B"`, "", 1), "SECRET-B") {
		t.Fatalf("teks bocor: %s", r.Text)
	}
	r = k.mcpCall(k.rawKey, "search_knowledge", map[string]any{"query": "rate", "project_id": k.projB})
	if !r.IsError || !strings.Contains(r.Text, "ditentukan oleh API key") {
		t.Fatalf("%s", r.Raw)
	}
}

func TestT08_Int_AuditSearch(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "search_knowledge", map[string]any{"query": "rate limit"})
	if r.IsError {
		t.Fatalf("%s", r.Raw)
	}
	var n int
	for i := 0; i < 50; i++ {
		if n = k.countAudit("mcp.search_knowledge"); n >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if n != 1 {
		t.Fatalf("entri audit = %d", n)
	}
	var targetType string
	var args, result []byte
	q := "SELECT target_type, args, result FROM " + k.schema + ".audit_log WHERE action = 'mcp.search_knowledge'"
	if err := k.admin.QueryRow(q).Scan(&targetType, &args, &result); err != nil {
		t.Fatal(err)
	}
	if targetType != "folder" {
		t.Fatalf("target_type=%q", targetType)
	}
	var res map[string]any
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatal(err)
	}
	if _, ok := res["count"]; !ok {
		t.Fatalf("result tanpa count: %s", result)
	}
	if _, ok := res["total"]; !ok {
		t.Fatalf("result tanpa total: %s", result)
	}
	if strings.Contains(string(result), "**") || strings.Contains(string(result), "rate limit") {
		t.Fatalf("result memuat snippet/query: %s", result)
	}
	if !strings.Contains(string(args), "rate limit") {
		t.Fatalf("args tanpa query: %s", args)
	}
}
