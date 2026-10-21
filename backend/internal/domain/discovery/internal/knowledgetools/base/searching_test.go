package base

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/knowledge/search"
)

func TestSearchHits_Text(t *testing.T) {
	h := NewSearchHits("rate limit", "", search.Response{
		Items: []search.Item{{Path: "docs/guides/b.md", Score: 0.6079, Snippet: "**rate**\n**limiting** per key"}}, Total: 1})
	want := "1 dari 1 hasil untuk \"rate limit\"\n1. docs/guides/b.md  (skor 0.608)\n   **rate** **limiting** per key\nBaca isi lengkap dengan read_file(path)."
	if got := h.Text(); got != want {
		t.Fatal(got)
	}
	h = NewSearchHits("x", "docs", search.Response{Items: []search.Item{{Path: "a", Score: 1}}, Total: 4})
	want = "1 dari 4 hasil untuk \"x\" di docs/\n1. a  (skor 1.000)\n[ada 3 hasil lain; persempit query atau path, atau naikkan limit (maks 20)]\nBaca isi lengkap dengan read_file(path)."
	if got := h.Text(); got != want {
		t.Fatal(got)
	}
}

func TestSearchHits_EmptyAndTotalFloor(t *testing.T) {
	r := NewSearchHits("xyz", "", search.Response{}).Result()
	if r.IsError || r.Text != "Tidak ada hasil untuk \"xyz\"\nCoba kata kunci lain yang lebih umum atau sinonim, kurangi jumlah kata, hapus filter path, atau jelajahi struktur dengan list_files." {
		t.Fatalf("%+v", r)
	}
	b, _ := json.Marshal(r.Structured)
	if string(b) != `{"items":[],"query":"xyz","total":0}` {
		t.Fatal(string(b))
	}
	if h := NewSearchHits("q", "", search.Response{Items: make([]search.Item, 3), Total: 1}); h.Total != 3 {
		t.Fatal(h.Total)
	}
	if got := NewSearchHits("q", "d", search.Response{}).Text(); got[:len("Tidak ada hasil untuk \"q\" di d/")] != "Tidak ada hasil untuk \"q\" di d/" {
		t.Fatal(got)
	}
}

func TestSearchHits_ResultAudit(t *testing.T) {
	r := NewSearchHits("rahasia", "docs", search.Response{Items: []search.Item{{Path: "p", Snippet: "s"}}, Total: 2}).Result()
	if r.TargetType != "folder" || r.Target != "docs" || len(r.Summary) != 3 || r.Summary["count"] != 1 || r.Summary["total"] != 2 {
		t.Fatalf("%+v", r)
	}
}

func hitsOf(n int, path func(i int) string, total int) SearchHits {
	items := make([]search.Item, n)
	for i := range items {
		items[i] = search.Item{Path: path(i), Score: 1, Snippet: "s"}
	}
	return NewSearchHits("q", "", search.Response{Items: items, Total: total})
}

func TestSummary_ResultPaths(t *testing.T) {
	h := hitsOf(3, func(i int) string { return "docs/" + string(rune('a'+i)) + ".md" }, 3)
	s := h.Summary()
	if got := s["result_paths"].([]string); len(got) != 3 || got[0] != "docs/a.md" || got[2] != "docs/c.md" {
		t.Fatalf("%v", got)
	}
	if s["count"] != 3 || s["total"] != 3 {
		t.Fatalf("%v", s)
	}
	if _, ok := s["result_paths_truncated"]; ok {
		t.Fatal("tidak boleh ada truncated")
	}

	s = hitsOf(15, func(i int) string { return "d/" + string(rune('a'+i)) }, 40).Summary()
	if len(s["result_paths"].([]string)) != 10 || s["count"] != 15 || s["total"] != 40 {
		t.Fatalf("%v", s)
	}
	if _, ok := s["result_paths_truncated"]; ok {
		t.Fatal("batas 10 bukan truncated")
	}
}

func TestSummary_EmptyIsJSONArray(t *testing.T) {
	s := NewSearchHits("x", "", search.Response{}).Summary()
	b, _ := json.Marshal(s)
	if string(b) != `{"count":0,"result_paths":[],"total":0}` {
		t.Fatal(string(b))
	}
}

func TestSummary_LongMultibytePathTruncated(t *testing.T) {
	long := strings.Repeat("é", 750) // 1500 byte
	paths, trunc := hitsOf(1, func(int) string { return long }, 1).ResultPaths()
	if trunc || len(paths) != 1 || len(paths[0]) > record.MaxTargetBytes || !utf8.ValidString(paths[0]) {
		t.Fatalf("len=%d trunc=%v", len(paths[0]), trunc)
	}
}

func TestSummary_Budget(t *testing.T) {
	h := hitsOf(10, func(i int) string { return strings.Repeat(string(rune('a'+i)), 1024) }, 10)
	s := h.Summary()
	paths := s["result_paths"].([]string)
	if s["result_paths_truncated"] != true || len(paths) >= 10 || len(paths) == 0 {
		t.Fatalf("n=%d %v", len(paths), s["result_paths_truncated"])
	}
	pb, _ := json.Marshal(paths)
	if len(pb) > ResultPathsBudgetBytes {
		t.Fatalf("budget %d", len(pb))
	}
	b, _ := json.Marshal(s)
	if len(b)+512 >= record.MaxResultJSONBytes {
		t.Fatalf("summary %d terlalu besar", len(b))
	}
}

func TestSummary_BudgetUsesEscapedLength(t *testing.T) {
	// "<" di-escape jadi 6 karakter; 1024 byte mentah -> ~6144 encoded, satu path saja muat.
	h := hitsOf(3, func(int) string { return strings.Repeat("<", 1024) }, 3)
	paths, trunc := h.ResultPaths()
	if !trunc || len(paths) != 0 && len(paths) != 1 {
		t.Fatalf("n=%d trunc=%v", len(paths), trunc)
	}
	pb, _ := json.Marshal(paths)
	if len(pb) > ResultPathsBudgetBytes {
		t.Fatalf("budget %d", len(pb))
	}
}

func TestSummary_NoLeakedKeys(t *testing.T) {
	s := hitsOf(2, func(i int) string { return "p" }, 2).Summary()
	for _, k := range []string{"snippet", "score", "content", "query", "items"} {
		if _, ok := s[k]; ok {
			t.Fatalf("kunci %s bocor", k)
		}
	}
}

func TestSearchHits_StructuredRegression(t *testing.T) {
	h := NewSearchHits("q", "", search.Response{Items: []search.Item{{Path: "a.md", Score: 0.5, Snippet: "x"}}, Total: 1})
	b, _ := json.Marshal(h.Structured())
	if string(b) != `{"items":[{"path":"a.md","score":0.5,"snippet":"x"}],"query":"q","total":1}` {
		t.Fatal(string(b))
	}
	if want := "1 dari 1 hasil untuk \"q\"\n1. a.md  (skor 0.500)\n   x\nBaca isi lengkap dengan read_file(path)."; h.Text() != want {
		t.Fatal(h.Text())
	}
}
