package base

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate_NotTruncated(t *testing.T) {
	out, tr, orig := Truncate("halo", 4)
	if out != "halo" || tr || orig != 4 {
		t.Fatal(out, tr, orig)
	}
}

func TestT07_AC7_Truncate500KB(t *testing.T) {
	text := strings.Repeat("a", 512000)
	out, tr, orig := Truncate(text, 102400)
	if !tr || orig != 512000 || len(out) > 102400 || !strings.Contains(out, "[truncated:") {
		t.Fatalf("tr=%v orig=%d len=%d", tr, orig, len(out))
	}
}

func TestTruncate_Multibyte(t *testing.T) {
	text := strings.Repeat("😀あ", 5000)
	for max := 200; max < 260; max++ {
		out, tr, _ := Truncate(text, max)
		if !tr || len(out) > max || !utf8.ValidString(out) {
			t.Fatalf("max=%d len=%d valid=%v", max, len(out), utf8.ValidString(out))
		}
	}
}

func TestTruncate_TinyMax(t *testing.T) {
	text := strings.Repeat("あ", 100)
	for _, max := range []int{0, 1, 5, 20} {
		out, tr, _ := Truncate(text, max)
		if !tr || len(out) > max || !utf8.ValidString(out) {
			t.Fatalf("max=%d out=%q", max, out)
		}
	}
}
