package stemid

import (
	"slices"
	"testing"
)

func TestVariants(t *testing.T) {
	cases := []struct {
		word     string
		contains []string
		absent   []string
	}{
		{"mengirim", []string{"kirim"}, nil},
		{"pengiriman", []string{"kiriman"}, nil},
		{"menulis", []string{"tulis"}, nil},
		{"memakai", []string{"pakai"}, nil},
		{"menyimpan", []string{"simpan"}, []string{"tyimpan"}},
		{"kirim", []string{"mengirim", "pengirim"}, nil},
		{"tulis", []string{"menulis", "penulis"}, nil},
		{"pakai", []string{"memakai", "pemakai"}, nil},
		{"simpan", []string{"menyimpan", "penyimpan"}, nil},
	}
	for _, c := range cases {
		got := Variants(c.word)
		for _, w := range c.contains {
			if !slices.Contains(got, w) {
				t.Errorf("Variants(%q) = %v, want contains %q", c.word, got, w)
			}
		}
		for _, w := range c.absent {
			if slices.Contains(got, w) {
				t.Errorf("Variants(%q) = %v, must not contain %q", c.word, got, w)
			}
		}
		if slices.Contains(got, c.word) {
			t.Errorf("Variants(%q) memuat kata asli", c.word)
		}
	}
}

func TestVariants_None(t *testing.T) {
	for _, w := range []string{"api", "deploy", "", "kyes", "ab"} {
		if got := Variants(w); got != nil {
			t.Errorf("Variants(%q) = %v, want nil", w, got)
		}
	}
}

func TestVariants_UniqueDeterministic(t *testing.T) {
	a, b := Variants("kirim"), Variants("kirim")
	if !slices.Equal(a, b) {
		t.Fatalf("tidak deterministik: %v vs %v", a, b)
	}
	seen := map[string]bool{}
	for _, v := range a {
		if seen[v] {
			t.Fatalf("duplikat %q", v)
		}
		seen[v] = true
	}
}
