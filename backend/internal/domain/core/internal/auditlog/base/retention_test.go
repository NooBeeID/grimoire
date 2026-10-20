package base

import (
	"testing"
	"time"
)

func TestPurgeCutoff(t *testing.T) {
	now := time.Date(2026, 10, 20, 3, 15, 0, 0, time.UTC)
	want := time.Date(2026, 7, 22, 3, 15, 0, 0, time.UTC)
	if got := PurgeCutoff(now, 90); !got.Equal(want) {
		t.Fatalf("got %v", got)
	}
	local := now.In(time.FixedZone("WIB", 7*3600))
	got := PurgeCutoff(local, 90)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("got %v", got)
	}
}
