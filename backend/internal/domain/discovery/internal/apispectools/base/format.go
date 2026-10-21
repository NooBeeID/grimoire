package base

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// FormatTime: "2006-01-02 15:04 UTC"; zero time -> "-" (K15).
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

var oneLineReplacer = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")

// OneLine meratakan pemisah baris dan tab menjadi spasi, merapatkan spasi beruntun, lalu TrimSpace.
func OneLine(s string) string {
	return strings.Join(strings.Fields(oneLineReplacer.Replace(s)), " ")
}

// Clip memotong s hingga maxRunes rune (termasuk "..." bila terpotong).
func Clip(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string([]rune(s)[:maxRunes])
	}
	return string([]rune(s)[:maxRunes-3]) + "..."
}

// JoinNames menggabungkan nama dengan ", "; lebih dari max -> "a, b, ... (+N lagi)".
func JoinNames(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + fmt.Sprintf(", ... (+%d lagi)", len(names)-max)
}
