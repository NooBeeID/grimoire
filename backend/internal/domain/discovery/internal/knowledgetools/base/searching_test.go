package base

import (
	"encoding/json"
	"testing"

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
	if r.TargetType != "folder" || r.Target != "docs" || len(r.Summary) != 2 || r.Summary["count"] != 1 || r.Summary["total"] != 2 {
		t.Fatalf("%+v", r)
	}
}
