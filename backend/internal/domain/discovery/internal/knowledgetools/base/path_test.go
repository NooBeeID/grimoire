package base

import (
	"strings"
	"testing"
)

func TestNormalizePrefix(t *testing.T) {
	ok := map[string]string{"  /docs/  ": "docs", "": "", "/": "", "//docs": "/docs", "docs/guides": "docs/guides", "a..b": "a..b"}
	for in, want := range ok {
		got, err := NormalizePrefix("path", in)
		if err != nil || got != want {
			t.Fatalf("%q -> %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"../a", "a/../b", "a\x00b", "a\tb", strings.Repeat("a", 1025), "\xff"} {
		if _, err := NormalizePrefix("path", in); err == nil {
			t.Fatalf("%q harus ditolak", in)
		}
	}
	if _, err := NormalizePrefix("path", strings.Repeat("a", 1024)); err != nil {
		t.Fatal(err.Message())
	}
	if _, err := NormalizePrefix("path", "../a"); err == nil || !strings.Contains(err.Message(), "tidak boleh memuat segmen ..") {
		t.Fatal(err)
	}
}

func TestNormalizePath(t *testing.T) {
	if got, err := NormalizePath("path", "/docs/a.md"); err != nil || got != "docs/a.md" {
		t.Fatal(got, err)
	}
	if got, err := NormalizePath("path", " docs/a.md "); err != nil || got != "docs/a.md" {
		t.Fatal(got, err)
	}
	if _, err := NormalizePath("path", "   "); err == nil || err.Reason != "wajib diisi" {
		t.Fatal(err)
	}
	if _, err := NormalizePath("path", "docs/"); err == nil || err.Reason != "menunjuk folder" || !strings.Contains(err.Message(), "list_files") {
		t.Fatal(err)
	}
	if _, err := NormalizePath("path", "a/../b"); err == nil {
		t.Fatal("want error")
	}
}
