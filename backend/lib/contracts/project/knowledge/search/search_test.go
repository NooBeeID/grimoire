package search

import "testing"

func TestLimits(t *testing.T) {
	if DefaultLimit != 20 || MaxLimit != 50 || DefaultLimit > MaxLimit {
		t.Fatalf("unexpected limits: %d/%d", DefaultLimit, MaxLimit)
	}
}
