//go:build integration

package server

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var ktMarkerRe = regexp.MustCompile(`\n\n\[terpotong: lanjutkan dengan offset=(\d+) dari total (\d+) byte\]$`)

func TestT08_Int_AC2_ReadChunks(t *testing.T) {
	k := setupKnowledgeTools(t)
	var buf strings.Builder
	offset, prev := 0, -1
	for i := 0; i < 100; i++ {
		res := k.mcpCall(k.rawKey, "read_file", map[string]any{"path": "docs/big.txt", "offset": offset, "limit": 50000})
		if res.IsError {
			t.Fatalf("isError: %s", res.Text)
		}
		if res.Meta["truncated"] != nil || res.StructuredContent != nil {
			t.Fatalf("meta/structured tak terduga: %s", res.Raw)
		}
		if len(res.Text) > 102400 {
			t.Fatalf("respons %d byte melebihi batas", len(res.Text))
		}
		m := ktMarkerRe.FindStringSubmatchIndex(res.Text)
		if m == nil {
			buf.WriteString(res.Text)
			if buf.String() != k.big {
				t.Fatalf("gabungan (%d byte) != asli (%d byte)", buf.Len(), len(k.big))
			}
			return
		}
		buf.WriteString(res.Text[:m[0]])
		next, _ := strconv.Atoi(res.Text[m[2]:m[3]])
		if next <= prev || next <= offset {
			t.Fatalf("offset tidak naik: %d setelah %d", next, offset)
		}
		prev, offset = offset, next
	}
	t.Fatal("terlalu banyak iterasi")
}

func TestT08_Int_AC3_SimilarPaths(t *testing.T) {
	k := setupKnowledgeTools(t)
	res := k.mcpCall(k.rawKey, "read_file", map[string]any{"path": "docs/redme.md"})
	if !res.IsError {
		t.Fatalf("want isError: %s", res.Text)
	}
	for _, s := range []string{"File tidak ditemukan: docs/redme.md", "Similar:", "- docs/readme.md", "list_files"} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("teks tidak memuat %q: %s", s, res.Text)
		}
	}
}

func TestT08_Int_AC5_ProjectArgRejected(t *testing.T) {
	k := setupKnowledgeTools(t)
	res := k.mcpCall(k.rawKey, "read_file", map[string]any{"path": "secret/b.md", "project_id": k.projB})
	if !res.IsError || !strings.Contains(res.Text, "ditentukan oleh API key") || strings.Contains(res.Text, "SECRET-B") {
		t.Fatalf("res=%+v", res)
	}
	res = k.mcpCall(k.rawKey, "read_file", map[string]any{"path": "secret/b.md"})
	if !res.IsError || !strings.Contains(res.Text, "File tidak ditemukan") || strings.Contains(res.Text, "SECRET-B") {
		t.Fatalf("res=%+v", res)
	}
}

func TestT08_Int_ReadOffsetBeyondEOF(t *testing.T) {
	k := setupKnowledgeTools(t)
	res := k.mcpCall(k.rawKey, "read_file", map[string]any{"path": "docs/a.md", "offset": 999999999})
	if !res.IsError || !strings.Contains(res.Text, "Permintaan ditolak:") ||
		!strings.Contains(res.Text, "Gunakan offset dari penanda terpotong atau 0.") {
		t.Fatalf("res=%+v", res)
	}
}
