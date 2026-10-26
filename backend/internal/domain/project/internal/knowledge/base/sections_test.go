package base

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func slice(c string, s Section) string { return string([]rune(c)[s.Start:s.End]) }

func TestSections(t *testing.T) {
	c := "intro teks\n\n# Satu\nisi satu\n## Dua\nisi dua\n```\n# bukan heading\n```\nSetext\n======\nakhir\n"
	got := Sections(c)
	var heads []string
	for _, s := range got {
		heads = append(heads, s.Heading)
	}
	if strings.Join(heads, "|") != "|Satu|Dua|Setext" {
		t.Fatalf("headings = %q", heads)
	}
	if !strings.HasPrefix(slice(c, got[0]), "intro teks") || !strings.HasPrefix(slice(c, got[1]), "# Satu") ||
		!strings.HasPrefix(slice(c, got[2]), "## Dua") || !strings.HasPrefix(slice(c, got[3]), "Setext\n===") {
		t.Fatalf("sections = %+v", got)
	}
	// Bagian bersambung tanpa celah sampai akhir isi.
	for i := 1; i < len(got); i++ {
		if got[i].Start != got[i-1].End {
			t.Fatalf("celah antara %d dan %d: %+v", i-1, i, got)
		}
	}
	if last := got[len(got)-1]; last.End != len([]rune(c)) {
		t.Fatalf("bagian terakhir berakhir di %d, want %d", last.End, len([]rune(c)))
	}
	if !strings.Contains(slice(c, got[2]), "# bukan heading") {
		t.Fatal("fenced code termasuk isi bagian Dua")
	}
}

func TestSections_NoPreambleWhenBlank(t *testing.T) {
	got := Sections("\n\n# A\nx")
	if len(got) != 1 || got[0].Heading != "A" {
		t.Fatalf("got %+v", got)
	}
}

func TestSections_NoHeadingAndEmpty(t *testing.T) {
	got := Sections("hanya teks")
	if len(got) != 1 || got[0] != (Section{"", 0, 10}) {
		t.Fatalf("got %+v", got)
	}
	if got := Sections(""); got == nil || len(got) != 0 {
		t.Fatalf("kosong = %#v", got)
	}
	if string(SectionsJSON("")) != "[]" {
		t.Fatalf("json kosong = %s", SectionsJSON(""))
	}
}

// Review Focus 2: offset adalah rune, bukan byte.
func TestSections_RuneOffsets(t *testing.T) {
	c := "😀😀\n# Judul\nisi"
	got := Sections(c)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[1].Heading != "Judul" || got[1].Start != 3 {
		t.Fatalf("Judul = %+v", got[1])
	}
	if !strings.HasPrefix(slice(c, got[1]), "# Judul") {
		t.Fatalf("slice = %q", slice(c, got[1]))
	}
	if slice(c, got[0]) != "😀😀\n" {
		t.Fatalf("preamble = %q", slice(c, got[0]))
	}
}

func TestSections_Cap(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "# H%d\nisi %d\n", i, i)
	}
	c := b.String()
	got := Sections(c)
	if len(got) != MaxSections {
		t.Fatalf("len = %d", len(got))
	}
	if got[MaxSections-1].End != len([]rune(c)) {
		t.Fatalf("bagian terakhir harus diperpanjang ke akhir: %+v", got[MaxSections-1])
	}
}

func TestSectionsJSON_Shape(t *testing.T) {
	var arr []map[string]any
	if err := json.Unmarshal(SectionsJSON("# A\nx"), &arr); err != nil || len(arr) != 1 ||
		arr[0]["h"] != "A" || arr[0]["s"] != float64(0) {
		t.Fatalf("json = %s %v", SectionsJSON("# A\nx"), err)
	}
}

func TestFile_SectionsJSONFilled(t *testing.T) {
	f := NewFile("p", "a.md", "# A\nx", Actor{ID: "u", Label: "U"})
	if !strings.Contains(string(f.SectionsJSON), `"h":"A"`) {
		t.Fatalf("NewFile sections = %s", f.SectionsJSON)
	}
	g := f.WithContent("# B\ny", Actor{ID: "u", Label: "U"})
	if !strings.Contains(string(g.SectionsJSON), `"h":"B"`) {
		t.Fatalf("WithContent sections = %s", g.SectionsJSON)
	}
}
