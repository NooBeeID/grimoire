package base

import (
	"slices"
	"testing"
)

func TestParseHeadline(t *testing.T) {
	got := ParseHeadline("a \x01login\x02 b")
	want := []Segment{{"a ", false}, {"login", true}, {" b", false}}
	if !slices.Equal(got, want) {
		t.Fatalf("%v", got)
	}
}

func TestParseHeadline_EdgeCases(t *testing.T) {
	cases := []struct {
		in   string
		want []Segment
	}{
		{"", []Segment{}},
		{"plain", []Segment{{"plain", false}}},
		{"x\x02y", []Segment{{"xy", false}}},
		{"x\x01y", []Segment{{"xy", false}}},
		{"\x01a\x02\x01b\x02", []Segment{{"ab", true}}},
		{"\x01\x02z", []Segment{{"z", false}}},
		{"\x01a\x01b\x02", []Segment{{"ab", true}}},
	}
	for _, tc := range cases {
		if got := ParseHeadline(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("ParseHeadline(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestToMarkdownBold(t *testing.T) {
	for in, want := range map[string]string{
		"Alur \x01login\x02 memakai": "Alur **login** memakai",
		"orphan\x02 and \x01 start":  "orphan and  start",
		"":                           "",
	} {
		if got := ToMarkdownBold(in); got != want {
			t.Errorf("ToMarkdownBold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClampLimit(t *testing.T) {
	for _, c := range [][4]int{{0, 20, 50, 20}, {-1, 20, 50, 20}, {5, 20, 50, 5}, {51, 20, 50, 50}, {50, 20, 50, 50}} {
		if got := ClampLimit(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("ClampLimit(%d,%d,%d)=%d want %d", c[0], c[1], c[2], got, c[3])
		}
	}
}

func TestNewSearchResult(t *testing.T) {
	r := NewSearchResult([]SearchHit{{Path: "a.md", Title: "A", Score: 0.5, Headline: "x \x01y\x02"}}, 3)
	if r.Total != 3 || len(r.Items) != 1 || len(r.Items[0].Snippet) != 2 || !r.Items[0].Snippet[1].Highlight {
		t.Fatalf("%+v", r)
	}
	if e := NewSearchResult(nil, 0); e.Items == nil {
		t.Fatal("items must be non-nil")
	}
}
