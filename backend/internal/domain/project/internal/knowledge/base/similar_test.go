package base

import (
	"slices"
	"testing"
)

func TestSimilarPaths(t *testing.T) {
	all := []string{"docs/auth/login.md", "docs/auth/logout.md", "z/login.md", "unrelated/very/long/other/file.txt", "docs/auth/logn.md"}
	got := SimilarPaths("docs/auth/logn.md", all, 5)
	if !slices.Contains(got, "docs/auth/login.md") || slices.Contains(got, "docs/auth/logn.md") ||
		slices.Contains(got, "unrelated/very/long/other/file.txt") {
		t.Fatalf("%v", got)
	}
}

func TestSimilarPaths_BasenameFirstAndLimit(t *testing.T) {
	all := []string{"docs/auth/login.mx", "a/Login.md", "b/login.md", "docs/auth/login.md"}
	got := SimilarPaths("docs/auth/login.md", all, 2)
	if !slices.Equal(got, []string{"a/Login.md", "b/login.md"}) {
		t.Fatalf("%v", got)
	}
	if SimilarPaths("x", all, 0) != nil {
		t.Fatal("n=0")
	}
}

func TestSimilarPaths_StableOrder(t *testing.T) {
	all := []string{"ab.md", "aa.md", "ac.md"}
	a := SimilarPaths("ad.md", all, 5)
	b := SimilarPaths("ad.md", []string{"ac.md", "aa.md", "ab.md"}, 5)
	if !slices.Equal(a, b) || !slices.Equal(a, []string{"aa.md", "ab.md", "ac.md"}) {
		t.Fatalf("%v %v", a, b)
	}
}

func TestLevenshtein(t *testing.T) {
	for _, c := range []struct {
		a, b string
		d    int
	}{{"", "abc", 3}, {"kitten", "sitting", 3}, {"é", "e", 1}, {"same", "same", 0}} {
		if got := levenshtein(c.a, c.b); got != c.d {
			t.Errorf("levenshtein(%q,%q)=%d want %d", c.a, c.b, got, c.d)
		}
	}
}
