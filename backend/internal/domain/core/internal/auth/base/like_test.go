package base

import "testing"

func TestLikePattern(t *testing.T) {
	cases := map[string]string{
		`50%_a\`: `%50\%\_a\\%`,
		"sari":   "%sari%",
		"":       "%%",
	}
	for in, want := range cases {
		if got := LikePattern(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
