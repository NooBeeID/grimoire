package base

import (
	"strings"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name, in string
		want     Reason
	}{
		{"ok", "docs/auth/login.md", ""},
		{"empty", "", ReasonPathEmpty},
		{"absolute", "/a.md", ReasonPathAbsolute},
		{"dotdot", "../x.md", ReasonPathTraversal},
		{"dot", "a/./b.md", ReasonPathTraversal},
		{"dotdot-mid", "a/../b.md", ReasonPathTraversal},
		{"double-slash", "a//b.md", ReasonPathEmptySegment},
		{"trailing-slash", "a/", ReasonPathEmptySegment},
		{"nul", "a\x00b.md", ReasonPathControlChar},
		{"newline", "a\nb.md", ReasonPathControlChar},
		{"600", strings.Repeat("a", 600), ReasonPathTooLong},
		{"513", strings.Repeat("a/", 256) + "ab", ReasonPathTooLong},
		{"512 ok", strings.Repeat("a/", 255) + "ab", ""},
		{"segment 256", strings.Repeat("a", 256), ReasonSegmentTooLong},
		{"segment 255", strings.Repeat("a", 255), ""},
		{"invalid utf8", "a\xffb.md", ReasonInvalidEncoding},
		{"case kept", "Docs/A.MD", ""},
		{"backslash ok", `a\b.md`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, r := NormalizePath(tc.in)
			if r != tc.want {
				t.Fatalf("reason = %q, want %q", r, tc.want)
			}
			if r == "" && got != tc.in {
				t.Fatalf("path changed: %q", got)
			}
		})
	}
}

func TestNormalizePrefix(t *testing.T) {
	cases := []struct {
		in, want string
		r        Reason
	}{
		{"", "", ""},
		{"docs", "docs", ""},
		{"docs/", "docs", ""},
		{"docs/auth", "docs/auth", ""},
		{"docs//", "", ReasonPathEmptySegment},
		{"/docs", "", ReasonPathAbsolute},
		{"/", "", ReasonPathAbsolute},
		{"a/../b", "", ReasonPathTraversal},
	}
	for _, tc := range cases {
		got, r := NormalizePrefix(tc.in)
		if got != tc.want || r != tc.r {
			t.Errorf("NormalizePrefix(%q) = %q,%q want %q,%q", tc.in, got, r, tc.want, tc.r)
		}
	}
}

func TestStripLeadingSlash(t *testing.T) {
	if StripLeadingSlash("/a") != "a" || StripLeadingSlash("//a") != "/a" || StripLeadingSlash("a") != "a" {
		t.Fatal("StripLeadingSlash")
	}
}

func TestIsHidden(t *testing.T) {
	for p, want := range map[string]bool{
		".git/config": true, "a/.DS_Store": true, "docs/.hidden/x.md": true, "docs/a.b.md": false, "a.md": false,
	} {
		if IsHidden(p) != want {
			t.Errorf("IsHidden(%q) != %v", p, want)
		}
	}
}

func TestExtAndSupported(t *testing.T) {
	for p, want := range map[string]bool{
		"a.md": true, "a.MD": true, "a.markdown": true, "a.txt": true,
		"a.yaml": false, "README": false, "x.md.bak": false, "dir.md/file": false,
	} {
		if IsSupported(p) != want {
			t.Errorf("IsSupported(%q) != %v", p, want)
		}
	}
	if Ext("x/A.MD") != ".md" || Ext("x/README") != "" {
		t.Fatal("Ext")
	}
}

func TestBaseDir(t *testing.T) {
	if Base("a/b/c.md") != "c.md" || Dir("a/b/c.md") != "a/b" || Base("c.md") != "c.md" || Dir("c.md") != "" {
		t.Fatal("Base/Dir")
	}
}

func TestLikePrefix(t *testing.T) {
	for in, want := range map[string]string{
		"":         "%",
		"docs":     "docs/%",
		`a_b%c\d`:  `a\_b\%c\\d/%`,
		"docs/api": "docs/api/%",
	} {
		if got := LikePrefix(in); got != want {
			t.Errorf("LikePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
