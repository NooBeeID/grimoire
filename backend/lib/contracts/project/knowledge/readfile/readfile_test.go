package readfile

import "testing"

func TestDefaultLimit(t *testing.T) {
	if DefaultLimit != 65536 {
		t.Fatalf("DefaultLimit = %d", DefaultLimit)
	}
}
