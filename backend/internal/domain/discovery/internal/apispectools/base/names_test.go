package base

import (
	"regexp"
	"testing"
)

func TestToolNames(t *testing.T) {
	re := regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	seen := map[string]bool{}
	for _, n := range []string{ToolListAPISpecs, ToolListAPIEndpoints, ToolGetAPIEndpoint} {
		if !re.MatchString(n) {
			t.Errorf("nama tool tidak valid: %q", n)
		}
		if seen[n] {
			t.Errorf("nama tool duplikat: %q", n)
		}
		seen[n] = true
	}
}

func TestNormalizeMethod(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{" get ", "GET", true},
		{"Patch", "PATCH", true},
		{"TRACE", "TRACE", true},
		{"CONNECT", "", false},
		{"GETS", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		got, ok := NormalizeMethod(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("NormalizeMethod(%q) = %q,%v; want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
