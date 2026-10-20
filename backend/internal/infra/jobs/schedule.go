package jobs

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var dailyAtRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// parseDailyAt memvalidasi "HH:MM" dan mengembalikan jam dan menit.
func parseDailyAt(s string) (hour, min int, err error) {
	if !dailyAtRe.MatchString(s) {
		return 0, 0, fmt.Errorf("invalid DailyAt %q, want HH:MM", s)
	}
	hour, _ = strconv.Atoi(s[:2])
	min, _ = strconv.Atoi(s[3:])
	return hour, min, nil
}

// nextRun mengembalikan waktu eksekusi berikutnya yang STRICTLY setelah now, selalu UTC.
//
//	Every:   kelipatan Every sejak Unix epoch yang > now
//	DailyAt: hari ini HH:MM:00 UTC bila > now, selain itu besok HH:MM:00 UTC
//
// Panic bila Job tidak valid (sudah divalidasi Register).
func nextRun(now time.Time, j Job) time.Time {
	now = now.UTC()
	if j.Every > 0 {
		e := int64(j.Every)
		n := (now.UnixNano()/e + 1) * e
		return time.Unix(0, n).UTC()
	}
	h, m, err := parseDailyAt(j.DailyAt)
	if err != nil {
		panic("jobs: nextRun: " + err.Error())
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, time.UTC)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}
