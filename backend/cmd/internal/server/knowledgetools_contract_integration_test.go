//go:build integration

package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestT08_Int_ToolsList(t *testing.T) {
	k := setupKnowledgeTools(t)
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
	for _, name := range []string{"list_files", "read_file", "search_knowledge"} {
		ro, ok := got[name]
		if !ok {
			t.Fatalf("tool %s tidak ada: %s", name, body)
		}
		if !ro {
			t.Fatalf("tool %s tanpa readOnlyHint:true: %s", name, body)
		}
	}
}

func TestT08_Int_AuditOneEntryPerCall(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "read_file", map[string]any{"path": "docs/a.md"})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	deadline := time.Now().Add(3 * time.Second)
	for k.countAudit("mcp.read_file") == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	if n := k.countAudit("mcp.read_file"); n != 1 {
		t.Fatalf("audit mcp.read_file = %d", n)
	}
	var targetType, target string
	var result []byte
	q := "SELECT coalesce(target_type,''), coalesce(target,''), result FROM " + k.schema + ".audit_log WHERE action = 'mcp.read_file'"
	if err := k.admin.QueryRow(q).Scan(&targetType, &target, &result); err != nil {
		t.Fatal(err)
	}
	if targetType != "file" || target != "docs/a.md" {
		t.Fatalf("target = %q/%q", targetType, target)
	}
	if strings.Contains(string(result), "isi a") {
		t.Fatalf("result memuat isi file: %s", result)
	}
}
