package jobs

import (
	"testing"
	"time"
)

func utc(y int, mo time.Month, d, h, mi, s, ns int) time.Time {
	return time.Date(y, mo, d, h, mi, s, ns, time.UTC)
}

func TestNextRun_DailyAt(t *testing.T) {
	jp := time.FixedZone("UTC+7", 7*3600)
	cases := []struct {
		name    string
		dailyAt string
		now     time.Time
		want    time.Time
	}{
		{"before", "03:00", utc(2026, 10, 20, 2, 59, 59, 0), utc(2026, 10, 20, 3, 0, 0, 0)},
		{"exact is strictly after", "03:00", utc(2026, 10, 20, 3, 0, 0, 0), utc(2026, 10, 21, 3, 0, 0, 0)},
		{"1ms after", "03:00", utc(2026, 10, 20, 3, 0, 0, 1e6), utc(2026, 10, 21, 3, 0, 0, 0)},
		{"cross day", "03:00", utc(2026, 10, 20, 23, 59, 59, 0), utc(2026, 10, 21, 3, 0, 0, 0)},
		{"year end", "03:00", utc(2026, 12, 31, 23, 0, 0, 0), utc(2027, 1, 1, 3, 0, 0, 0)},
		{"non-UTC now", "03:00", time.Date(2026, 10, 20, 9, 59, 0, 0, jp), utc(2026, 10, 20, 3, 0, 0, 0)},
		{"non-UTC now after", "03:00", time.Date(2026, 10, 20, 10, 1, 0, 0, jp), utc(2026, 10, 21, 3, 0, 0, 0)},
		{"midnight", "00:00", utc(2026, 10, 20, 12, 0, 0, 0), utc(2026, 10, 21, 0, 0, 0, 0)},
		{"23:59", "23:59", utc(2026, 10, 20, 12, 0, 0, 0), utc(2026, 10, 20, 23, 59, 0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nextRun(c.now, Job{DailyAt: c.dailyAt})
			if !got.Equal(c.want) || got.Location() != time.UTC {
				t.Fatalf("got %v (%v), want %v", got, got.Location(), c.want)
			}
		})
	}
}

func TestNextRun_Every(t *testing.T) {
	j := Job{Every: 15 * time.Minute}
	if got, want := nextRun(utc(2026, 10, 20, 10, 7, 0, 0), j), utc(2026, 10, 20, 10, 15, 0, 0); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got, want := nextRun(utc(2026, 10, 20, 10, 15, 0, 0), j), utc(2026, 10, 20, 10, 30, 0, 0); !got.Equal(want) {
		t.Fatalf("exact: got %v want %v", got, want)
	}
	a := nextRun(utc(2026, 10, 20, 10, 16, 0, 0), j)
	b := nextRun(utc(2026, 10, 20, 10, 29, 59, 0), j)
	if !a.Equal(b) {
		t.Fatalf("same tick differs: %v vs %v", a, b)
	}
	if a.UnixNano()%int64(j.Every) != 0 {
		t.Fatal("not a multiple of Every since epoch")
	}
	for _, e := range []time.Duration{time.Second, 24 * time.Hour} {
		now := utc(2026, 10, 20, 10, 7, 3, 500)
		got := nextRun(now, Job{Every: e})
		if !got.After(now) || got.UnixNano()%int64(e) != 0 || got.Sub(now) > e {
			t.Fatalf("Every %v: got %v", e, got)
		}
	}
}

func TestParseDailyAt(t *testing.T) {
	for _, s := range []string{"00:00", "09:05", "23:59"} {
		if _, _, err := parseDailyAt(s); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
	h, m, _ := parseDailyAt("09:05")
	if h != 9 || m != 5 {
		t.Errorf("got %d:%d", h, m)
	}
	for _, s := range []string{"24:00", "12:60", "9:05", "12:5", "", "12:00:00", "ab:cd"} {
		if _, _, err := parseDailyAt(s); err == nil {
			t.Errorf("%q should be invalid", s)
		}
	}
}
