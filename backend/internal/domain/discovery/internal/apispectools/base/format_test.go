package base

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFormatTime(t *testing.T) {
	if FormatTime(time.Time{}) != "-" {
		t.Fatal("zero")
	}
	jkt := time.FixedZone("Asia/Jakarta", 7*3600)
	if got := FormatTime(time.Date(2026, 10, 20, 15, 15, 0, 0, jkt)); got != "2026-10-20 08:15 UTC" {
		t.Fatalf("got %q", got)
	}
}

func TestOneLine(t *testing.T) {
	if got := OneLine("A\r\nB\tC   D\n"); got != "A B C D" {
		t.Fatalf("got %q", got)
	}
	if OneLine(" \n\t ") != "" {
		t.Fatal("blank")
	}
}

func TestClip(t *testing.T) {
	if Clip("abc", 5) != "abc" {
		t.Fatal("short")
	}
	got := Clip(strings.Repeat("é", 200), 120)
	if utf8.RuneCountInString(got) != 120 || !strings.HasSuffix(got, "...") || !utf8.ValidString(got) {
		t.Fatalf("got %q", got)
	}
	if Clip("abcdef", 3) != "abc" {
		t.Fatal("tiny")
	}
}

func TestJoinNames(t *testing.T) {
	if JoinNames(nil, 20) != "" {
		t.Fatal("empty")
	}
	if JoinNames([]string{"a", "b", "c"}, 20) != "a, b, c" {
		t.Fatal("three")
	}
	var many []string
	for i := 0; i < 25; i++ {
		many = append(many, "n")
	}
	got := JoinNames(many, 20)
	if !strings.HasSuffix(got, "... (+5 lagi)") || strings.Count(got, "n") != 20 {
		t.Fatalf("got %q", got)
	}
}

func TestSpecVersionLabel(t *testing.T) {
	for in, want := range map[string]string{"2.0": "Swagger 2.0", "3.0.3": "OpenAPI 3.0.3", "": "OpenAPI -", "3.1.0": "OpenAPI 3.1.0"} {
		if got := SpecVersionLabel(in); got != want {
			t.Fatalf("%q: %q", in, got)
		}
	}
}
