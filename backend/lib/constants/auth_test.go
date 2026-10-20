package constants

import (
	"regexp"
	"testing"
)

func TestPasswordGateExempt_KeyFormat(t *testing.T) {
	re := regexp.MustCompile(`^(GET|POST|PATCH|PUT|DELETE) /api/\S+$`)
	if len(PasswordGateExempt) != 4 {
		t.Fatalf("harus 4 route exempt, got %d", len(PasswordGateExempt))
	}
	for k := range PasswordGateExempt {
		if !re.MatchString(k) {
			t.Errorf("format key salah: %q", k)
		}
	}
}
