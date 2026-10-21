package base

import "testing"

func TestLikePattern(t *testing.T) {
	if got := LikePattern(`50%_a\b`); got != `%50\%\_a\\b%` {
		t.Fatal(got)
	}
}
