package base

import "time"

// PurgeCutoff menghitung batas waktu: entri dengan occurred_at lebih lama dihapus.
func PurgeCutoff(now time.Time, days int) time.Time {
	return now.UTC().Add(-time.Duration(days) * 24 * time.Hour)
}
