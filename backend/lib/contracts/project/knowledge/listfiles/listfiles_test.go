package listfiles

import "testing"

func TestLimits(t *testing.T) {
	if DefaultLimit != 100 || MaxLimit != 500 || DefaultLimit > MaxLimit {
		t.Fatalf("unexpected limits: %d/%d", DefaultLimit, MaxLimit)
	}
}
