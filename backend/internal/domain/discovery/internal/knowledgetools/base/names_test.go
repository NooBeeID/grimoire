package base

import (
	"regexp"
	"testing"
)

func TestToolNames(t *testing.T) {
	re := regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	seen := map[string]bool{}
	for _, n := range []string{ToolListFiles, ToolReadFile, ToolSearchKnowledge, ToolGetFileOutline, ToolReadSection, ToolListRecentChanges} {
		if !re.MatchString(n) {
			t.Fatalf("nama tidak valid: %s", n)
		}
		if seen[n] {
			t.Fatalf("duplikat: %s", n)
		}
		seen[n] = true
	}
}
