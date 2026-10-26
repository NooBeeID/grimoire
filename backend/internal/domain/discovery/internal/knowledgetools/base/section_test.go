package base

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/lib/util/mdoutline"
)

func firstHeading(t *testing.T, content, text string, occ int) mdoutline.Heading {
	t.Helper()
	h, _, ok := mdoutline.Find(mdoutline.Parse(content), text, occ)
	if !ok {
		t.Fatalf("heading %q tidak ada", text)
	}
	return h
}

func TestT18_AC2_SectionExact(t *testing.T) {
	c := "# A\nx\n## A1\ny\n# B\nz\n"
	s := NewSection("a.md", c, firstHeading(t, c, "A", 1), 1, 40000)
	if s.Content != "# A\nx\n## A1\ny\n" || strings.Contains(s.Content, "# B") || s.Truncated || s.NextOffset != -1 {
		t.Fatalf("%+v", s)
	}
}

func TestT18_AC2_SectionUTF8Cut(t *testing.T) {
	c := "# A\n" + strings.Repeat("日本語", 50) + "\n# B\n"
	h := firstHeading(t, c, "A", 1)
	s := NewSection("a.md", c, h, 1, 8) // 4 + 1 byte masuk rune kedua
	if !utf8.ValidString(s.Content) || len(s.Content) > 8 || !s.Truncated || s.NextOffset != h.Start+len(s.Content) {
		t.Fatalf("%+v", s)
	}
	if s.Content != "# A\n日" {
		t.Fatalf("content = %q", s.Content)
	}
	if !strings.Contains(s.Result().Text, "\n\n[bagian terpotong: lanjutkan dengan read_file offset=7; bagian berakhir di byte ") {
		t.Fatalf("text = %q", s.Result().Text)
	}
}

func TestNewSection_LastSectionToEOF(t *testing.T) {
	c := "# A\nx\n# B\nlast"
	s := NewSection("a.md", c, firstHeading(t, c, "B", 1), 1, 40000)
	if s.Content != "# B\nlast" || s.Truncated {
		t.Fatalf("%+v", s)
	}
}

func TestNewSection_CRLF(t *testing.T) {
	c := "# A\r\nx\r\n# B\r\n"
	s := NewSection("a.md", c, firstHeading(t, c, "A", 1), 1, 40000)
	if s.Content != "# A\r\nx\r\n" {
		t.Fatalf("content = %q", s.Content)
	}
}

func TestSection_ResultShape(t *testing.T) {
	c := "intro\n# A\nx\n"
	s := NewSection("docs/a.md", c, firstHeading(t, c, "A", 1), 1, 40000)
	r := s.Result()
	if r.IsError || r.TargetType != "file" || r.Target != "docs/a.md" || r.Text != "# A\nx\n" {
		t.Fatalf("%+v", r)
	}
	if r.Summary["count"] != 1 || r.Summary["bytes"] != 6 || r.Summary["truncated"] != false || r.Summary["occurrence"] != 1 {
		t.Fatalf("summary = %v", r.Summary)
	}
	for k, v := range r.Summary {
		if _, ok := v.(string); ok {
			t.Fatalf("string di %s", k)
		}
	}
	if r.Structured["start"] != 6 || r.Structured["end"] != 12 || r.Structured["heading"] != "A" || r.Structured["next_offset"] != -1 {
		t.Fatalf("structured = %v", r.Structured)
	}
	if _, err := json.Marshal(r.Structured); err != nil {
		t.Fatal(err)
	}
}

func TestSimilarHeadings(t *testing.T) {
	hs := mdoutline.Parse("# Install\n# Setup\n# Setup Guide\n")
	got := SimilarHeadings(hs, "setup", 5)
	if len(got) != 2 || got[0] != "# Setup (L2)" || got[1] != "# Setup Guide (L3)" {
		t.Fatalf("got %v", got)
	}
	got = SimilarHeadings(hs, "zzz", 5)
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
	words := SimilarHeadings(mdoutline.Parse("## Panduan Setup Awal\n"), "setup lanjutan", 5)
	if len(words) != 1 || words[0] != "## Panduan Setup Awal (L1)" {
		t.Fatalf("words %v", words)
	}
	if got := SimilarHeadings(mdoutline.Parse("# a\n# a1\n# a2\n"), "a", 2); len(got) != 2 {
		t.Fatalf("max %v", got)
	}
}

func TestHeadingMiss_Text(t *testing.T) {
	m := HeadingMiss{Path: "docs/a.md", Heading: "Foo", Occurrence: 1, Similar: []string{"# Foobar (L3)"}}
	r := m.Result()
	for _, w := range []string{`Heading tidak ditemukan di docs/a.md: "Foo"`, "Mirip:", "- # Foobar (L3)", "Panggil get_file_outline untuk melihat semua heading."} {
		if !strings.Contains(r.Text, w) {
			t.Errorf("text tanpa %q:\n%s", w, r.Text)
		}
	}
	if !r.IsError || r.Summary["error_code"] != "heading_not_found" || r.Summary["similar_count"] != 1 || r.Summary["count"] != 0 {
		t.Fatalf("%+v", r)
	}
	m2 := HeadingMiss{Path: "docs/a.md", Heading: "Setup", Occurrence: 3, Count: 2}
	if got := m2.Result().Text; got != `Heading "Setup" hanya muncul 2 kali di docs/a.md; gunakan occurrence 1 sampai 2.` {
		t.Fatalf("text = %q", got)
	}
}

func TestNotMarkdown_Result(t *testing.T) {
	r := NotMarkdown{Path: "a.txt"}.Result()
	if !r.IsError || r.Summary["error_code"] != "not_markdown" ||
		r.Text != "read_section hanya untuk file Markdown (.md, .markdown): a.txt. Baca dengan read_file." {
		t.Fatalf("%+v", r)
	}
}
