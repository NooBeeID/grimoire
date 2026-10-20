package base

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUA(t *testing.T) {
	if TruncateUA("abc") != "abc" {
		t.Fatal("pendek")
	}
	if got := TruncateUA(strings.Repeat("a", 600)); len(got) != MaxUserAgentBytes {
		t.Fatalf("len %d", len(got))
	}
	got := TruncateUA(strings.Repeat("é", 400)) // 800 byte, rune 2 byte
	if len(got) > MaxUserAgentBytes || !utf8.ValidString(got) {
		t.Fatalf("len %d valid=%v", len(got), utf8.ValidString(got))
	}
}
