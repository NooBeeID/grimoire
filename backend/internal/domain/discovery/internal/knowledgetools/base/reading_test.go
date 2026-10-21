package base

import (
	"strings"
	"testing"

	"grimoire/lib/contracts/project/knowledge/readfile"
)

func TestReadLimits(t *testing.T) {
	if MaxReadLimit(102400) != 101888 || MaxReadLimit(1024) != 1024 || MaxReadLimit(100) != 1024 {
		t.Fatal("MaxReadLimit")
	}
	if DefaultReadLimit(101888) != 40000 || DefaultReadLimit(1024) != 1024 {
		t.Fatal("DefaultReadLimit")
	}
}

func TestReadChunk_Text(t *testing.T) {
	cases := []struct {
		name string
		resp readfile.Response
		off  int
		want string
	}{
		{"utuh", readfile.Response{Path: "a", Content: "halo\n", SizeBytes: 5, TotalBytes: 5, NextOffset: 5}, 0, "halo\n"},
		{"terpotong", readfile.Response{Path: "a", Content: "abc", SizeBytes: 3, TotalBytes: 10, NextOffset: 3, Truncated: true}, 0,
			"abc\n\n[terpotong: lanjutkan dengan offset=3 dari total 10 byte]"},
		{"lanjutan tanpa penanda", readfile.Response{Path: "a", Content: "xyz", SizeBytes: 3, TotalBytes: 10, NextOffset: 10}, 7, "xyz"},
		{"kosong", readfile.Response{Path: "a/empty"}, 0, "[file kosong: a/empty]"},
		{"di akhir", readfile.Response{Path: "a", TotalBytes: 10, NextOffset: 10}, 10, "[tidak ada isi pada offset=10; file berukuran 10 byte]"},
	}
	for _, tc := range cases {
		if got := NewReadChunk(tc.resp, tc.off).Text(); got != tc.want {
			t.Errorf("%s: %q", tc.name, got)
		}
	}
}

func TestReadChunk_ResultAndSummary(t *testing.T) {
	r := NewReadChunk(readfile.Response{Path: "docs/a.md", Content: "SECRET", SizeBytes: 6, TotalBytes: 20, NextOffset: 6, Truncated: true}, 0).Result()
	if r.IsError || r.Structured != nil || r.TargetType != "file" || r.Target != "docs/a.md" {
		t.Fatalf("%+v", r)
	}
	if r.Summary["next_offset"] != 6 || r.Summary["truncated"] != true || r.Summary["bytes"] != 6 {
		t.Fatalf("%v", r.Summary)
	}
	for _, v := range r.Summary {
		if s, ok := v.(string); ok && strings.Contains(s, "SECRET") {
			t.Fatal("isi bocor")
		}
	}
	done := NewReadChunk(readfile.Response{Path: "a", Content: "x", SizeBytes: 1, TotalBytes: 1, NextOffset: 1}, 0).Summary()
	if done["next_offset"] != -1 {
		t.Fatal(done)
	}
}

func TestNotFound(t *testing.T) {
	n := NewNotFound("docs/redme.md", []string{"1", "2", "3", "4", "5", "6"})
	if len(n.Similar) != 5 {
		t.Fatal(n.Similar)
	}
	n = NewNotFound("docs/redme.md", []string{"docs/readme.md", "readme.md"})
	want := "File tidak ditemukan: docs/redme.md\nSimilar:\n- docs/readme.md\n- readme.md\nGunakan list_files untuk melihat struktur folder atau search_knowledge untuk mencari berdasarkan isi."
	if n.Text() != want {
		t.Fatal(n.Text())
	}
	r := n.Result()
	if !r.IsError || r.Summary["similar_count"] != 2 || r.Summary["error_code"] != "not_found" || r.Summary["found"] != false {
		t.Fatalf("%+v", r)
	}
	empty := NewNotFound("x", nil)
	if strings.Contains(empty.Text(), "Similar:") || !strings.HasPrefix(empty.Text(), "File tidak ditemukan: x\nGunakan") {
		t.Fatal(empty.Text())
	}
}
