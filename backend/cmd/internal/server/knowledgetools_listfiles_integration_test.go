//go:build integration

package server

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"
)

func ktItemPaths(t *testing.T, r ktResult) []string {
	t.Helper()
	items, _ := r.StructuredContent["items"].([]any)
	var out []string
	for _, it := range items {
		out = append(out, it.(map[string]any)["path"].(string))
	}
	return out
}

func TestT08_Int_AC1_ListTopLevel(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "list_files", map[string]any{})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	if !strings.Contains(r.Text, "readme.md") || !strings.Contains(r.Text, "[folder] docs/") {
		t.Fatalf("teks: %s", r.Text)
	}
	if strings.Contains(r.Text, "docs/a.md") || strings.Contains(r.Text, "docs/guides/b.md") {
		t.Fatalf("tidak boleh rekursif: %s", r.Text)
	}
	if strings.Contains(r.Text, "[ada ") {
		t.Fatalf("tidak boleh ada penanda: %s", r.Text)
	}
	folders, _ := r.StructuredContent["folders"].([]any)
	if len(folders) != 1 || folders[0] != "docs" {
		t.Fatalf("folders: %v", r.StructuredContent["folders"])
	}
	if v, ok := r.StructuredContent["next_offset"]; !ok || v != nil {
		t.Fatalf("next_offset: %v", v)
	}
}

func TestT08_Int_AC1_ListRecursivePaged(t *testing.T) {
	k := setupKnowledgeTools(t)
	var all []string
	offset := 0
	for page := 0; page < 10; page++ {
		r := k.mcpCall(k.rawKey, "list_files", map[string]any{"recursive": true, "limit": 2, "offset": offset})
		if r.IsError {
			t.Fatalf("isError: %s", r.Raw)
		}
		if strings.Contains(r.Text, "[folder]") {
			t.Fatalf("rekursif tanpa [folder]: %s", r.Text)
		}
		paths := ktItemPaths(t, r)
		all = append(all, paths...)
		next := r.StructuredContent["next_offset"]
		if page == 0 {
			if len(paths) != 2 || next != float64(2) || !strings.Contains(r.Text, "offset=2") {
				t.Fatalf("halaman 1: %v next=%v\n%s", paths, next, r.Text)
			}
		}
		if next == nil {
			if strings.Contains(r.Text, "[ada ") {
				t.Fatalf("halaman akhir tanpa penanda: %s", r.Text)
			}
			break
		}
		offset = int(next.(float64))
	}
	want := []string{"docs/a.md", "docs/big.txt", "docs/guides/b.md", "docs/readme.md", "readme.md"}
	got := append([]string(nil), all...)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("gabungan %v want %v", all, want)
	}
}

func TestT08_Int_ListFiles_PrefixAndEmpty(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "list_files", map[string]any{"path": "docs"})
	if r.IsError || !strings.Contains(r.Text, "docs/a.md") || !strings.Contains(r.Text, "[folder] docs/guides/") {
		t.Fatalf("docs: %s", r.Text)
	}
	if strings.Contains(r.Text, "docs/guides/b.md") {
		t.Fatalf("non-rekursif: %s", r.Text)
	}
	r = k.mcpCall(k.rawKey, "list_files", map[string]any{"path": "tidak-ada"})
	if r.IsError || !strings.Contains(r.Text, "Tidak ada file di folder 'tidak-ada'") {
		t.Fatalf("kosong: %s", r.Raw)
	}
}

func TestT08_Int_ListFiles_OffsetPastEnd(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "list_files", map[string]any{"offset": 999})
	if !r.IsError || !strings.Contains(r.Text, "Offset 999 melewati jumlah file (") {
		t.Fatalf("got: %s", r.Raw)
	}
}

func TestT08_Int_AC5_ListFilesProjectArgRejected(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "list_files", map[string]any{"project_id": k.projB, "recursive": true})
	if !r.IsError || !strings.Contains(r.Text, "ditentukan oleh API key") {
		t.Fatalf("got: %s", r.Raw)
	}
	if strings.Contains(r.Raw, "secret/b.md") {
		t.Fatalf("bocor: %s", r.Raw)
	}
	// Tanpa argumen project, hanya file project A yang tampak.
	r = k.mcpCall(k.rawKey, "list_files", map[string]any{"recursive": true})
	if strings.Contains(r.Raw, "secret/b.md") {
		t.Fatalf("bocor lintas project: %s", r.Raw)
	}
}

func TestT08_Int_ListFiles_Audit(t *testing.T) {
	k := setupKnowledgeTools(t)
	r := k.mcpCall(k.rawKey, "list_files", map[string]any{})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	deadline := time.Now().Add(3 * time.Second)
	for k.countAudit("mcp.list_files") == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := k.countAudit("mcp.list_files"); n != 1 {
		t.Fatalf("audit mcp.list_files = %d", n)
	}
	var targetType string
	var result []byte
	q := "SELECT coalesce(target_type,''), result FROM " + k.schema + ".audit_log WHERE action = 'mcp.list_files'"
	if err := k.admin.QueryRow(q).Scan(&targetType, &result); err != nil {
		t.Fatal(err)
	}
	if targetType != "folder" {
		t.Fatalf("target_type = %q", targetType)
	}
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"count", "total", "folders", "next_offset", "recursive"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("result tanpa %s: %s", key, result)
		}
	}
	if strings.Contains(string(result), "readme.md") || strings.Contains(string(result), "docs") {
		t.Fatalf("result memuat path: %s", result)
	}
}
