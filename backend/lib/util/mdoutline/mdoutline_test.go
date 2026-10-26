package mdoutline

import (
	"os"
	"reflect"
	"testing"
)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func texts(hs []Heading) []string {
	out := []string{}
	for _, h := range hs {
		out = append(out, h.Text)
	}
	return out
}

func levels(hs []Heading) []int {
	out := []int{}
	for _, h := range hs {
		out = append(out, h.Level)
	}
	return out
}

func TestT18_AC1_FenceIgnored(t *testing.T) {
	hs := Parse(read(t, "fence.md"))
	if !reflect.DeepEqual(texts(hs), []string{"Real", "After"}) {
		t.Fatalf("texts = %v", texts(hs))
	}
	if !reflect.DeepEqual(levels(hs), []int{1, 2}) {
		t.Fatalf("levels = %v", levels(hs))
	}
}

func TestT18_AC1_Setext(t *testing.T) {
	hs := Parse(read(t, "setext.md"))
	if len(hs) != 2 {
		t.Fatalf("len = %d: %+v", len(hs), hs)
	}
	if hs[0].Level != 1 || hs[0].Text != "Judul" || hs[0].Line != 1 || hs[0].Start != 0 {
		t.Fatalf("h0 = %+v", hs[0])
	}
	if hs[1].Level != 2 || hs[1].Text != "Sub" || hs[1].Line != 4 {
		t.Fatalf("h1 = %+v", hs[1])
	}
}

func TestParse_SetextMultiLine(t *testing.T) {
	hs := Parse("a\nb\n===\n")
	if len(hs) != 1 || hs[0].Level != 1 || hs[0].Text != "a b" || hs[0].Line != 1 || hs[0].Start != 0 {
		t.Fatalf("hs = %+v", hs)
	}
}

func TestT18_AC1_DuplicateHeadings(t *testing.T) {
	hs := Parse(read(t, "duplicate.md"))
	h, count, ok := Find(hs, "Setup", 2)
	if !ok || count != 2 || h.Line != 5 {
		t.Fatalf("h=%+v count=%d ok=%v", h, count, ok)
	}
	_, count, ok = Find(hs, "Setup", 3)
	if ok || count != 2 {
		t.Fatalf("count=%d ok=%v", count, ok)
	}
}

func TestT18_AC1_NoHeadings(t *testing.T) {
	for _, c := range []string{read(t, "noheading.md"), ""} {
		hs := Parse(c)
		if hs == nil || len(hs) != 0 {
			t.Fatalf("hs = %#v", hs)
		}
	}
}

func TestParse_SectionEnds(t *testing.T) {
	c := "# A\nx\n## A1\ny\n# B\nz\n"
	hs := Parse(c)
	if len(hs) != 3 {
		t.Fatalf("hs = %+v", hs)
	}
	a, a1, b := hs[0], hs[1], hs[2]
	if a.End != b.Start || a1.End != b.Start || b.End != len(c) {
		t.Fatalf("ends: %+v", hs)
	}
}

func TestParse_ATXEdgeCases(t *testing.T) {
	for _, c := range []string{"#tag\n", "#!/bin/sh\n", "    # code\n", "\t# code\n", "####### seven\n"} {
		if hs := Parse(c); len(hs) != 0 {
			t.Fatalf("%q -> %+v", c, hs)
		}
	}
	hs := Parse("  ## Two ##  \n")
	if len(hs) != 1 || hs[0].Text != "Two" || hs[0].Level != 2 {
		t.Fatalf("hs = %+v", hs)
	}
	hs = Parse("#")
	if len(hs) != 1 || hs[0].Text != "" || hs[0].Level != 1 {
		t.Fatalf("hs = %+v", hs)
	}
}

func TestParse_HTMLBlockIgnored(t *testing.T) {
	hs := Parse(read(t, "html.md"))
	if !reflect.DeepEqual(texts(hs), []string{"yes"}) {
		t.Fatalf("texts = %v", texts(hs))
	}
}

func TestParse_FrontMatterSkipped(t *testing.T) {
	hs := Parse(read(t, "frontmatter.md"))
	if len(hs) != 1 || hs[0].Text != "Real" || hs[0].Line != 4 {
		t.Fatalf("hs = %+v", hs)
	}
}

func TestParse_ThematicBreakNotSetext(t *testing.T) {
	if hs := Parse("a\n\n---\n- item\n---\n"); len(hs) != 0 {
		t.Fatalf("hs = %+v", hs)
	}
}

func TestParse_CRLFAndBOM(t *testing.T) {
	c := "\ufeff# A\r\nx\r\n## B\r\n"
	hs := Parse(c)
	if len(hs) != 2 || hs[0].Text != "A" || hs[1].Text != "B" {
		t.Fatalf("hs = %+v", hs)
	}
	if hs[0].Start != 0 || hs[1].Start != len("\ufeff# A\r\nx\r\n") {
		t.Fatalf("starts: %+v", hs)
	}
	if hs[0].End != len(c) || hs[1].End != len(c) {
		t.Fatalf("ends: %+v", hs)
	}
}

func TestFind_TrimsArgument(t *testing.T) {
	hs := Parse("# Intro\n")
	if _, _, ok := Find(hs, "  Intro ", 1); !ok {
		t.Fatal("not found")
	}
}

func TestParse_UnclosedFenceToEOF(t *testing.T) {
	if hs := Parse("```\n# x\n"); len(hs) != 0 {
		t.Fatalf("hs = %+v", hs)
	}
}

func TestParse_FrontMatterUnclosedIsNotFrontMatter(t *testing.T) {
	// tanpa penutup: baris 1 hanya thematic break, "# A" tetap heading.
	hs := Parse("---\n# A\n")
	if len(hs) != 1 || hs[0].Text != "A" {
		t.Fatalf("hs = %+v", hs)
	}
}
