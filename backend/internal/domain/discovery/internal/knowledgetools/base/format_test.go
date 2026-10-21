package base

import (
	"testing"
	"time"
)

func TestHumanSize(t *testing.T) {
	cases := map[int]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1536: "1.5 KB", 1048575: "1024.0 KB", 1048576: "1.0 MB", 2411724: "2.3 MB", 1 << 30: "1.0 GB"}
	for in, want := range cases {
		if got := HumanSize(in); got != want {
			t.Errorf("%d -> %q, want %q", in, got, want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	ts := time.Date(2026, 10, 20, 15, 15, 0, 0, jkt)
	if got := FormatTime(ts); got != "2026-10-20 08:15 UTC" {
		t.Fatal(got)
	}
	if got := RFC3339(ts); got != "2026-10-20T08:15:00Z" {
		t.Fatal(got)
	}
	if FormatTime(time.Time{}) != "-" || RFC3339(time.Time{}) != "" {
		t.Fatal("zero time")
	}
}

func TestOneLine(t *testing.T) {
	if got := OneLine("a\r\nb\nc\rd\te"); got != "a b c d e" {
		t.Fatal(got)
	}
}
