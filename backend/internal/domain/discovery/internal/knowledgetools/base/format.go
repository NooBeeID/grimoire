package base

import (
	"fmt"
	"strings"
	"time"
)

// HumanSize memformat ukuran byte (basis 1024, satu desimal) sesuai D9.
func HumanSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	unit := "KB"
	for _, u := range []string{"KB", "MB", "GB"} {
		f /= 1024
		unit = u
		if f < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", f, unit)
}

// FormatTime: "2006-01-02 15:04 UTC"; zero time -> "-".
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// RFC3339: UTC; zero time -> "".
func RFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

var oneLineReplacer = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")

// OneLine mengganti pemisah baris dan tab dengan satu spasi.
func OneLine(s string) string { return oneLineReplacer.Replace(s) }
