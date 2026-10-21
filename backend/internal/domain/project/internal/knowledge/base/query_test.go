package base

import (
	"strings"
	"testing"
)

func TestParseQuery(t *testing.T) {
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"word", "login", `"login"`, true},
		{"two words", "reset password", `"reset" "password"`, true},
		{"phrase", `"reset password"`, `"reset password"`, true},
		{"unclosed", `"unclosed phrase`, `"unclosed phrase"`, true},
		{"operators", `a & b : c or -d !e`, `"a" "b" "c" "or" "d" "e"`, true},
		{"quotes only", `"""`, "", false},
		{"symbols only", "&&&", "", false},
		{"spaces", "   ", "", false},
		{"empty", "", "", false},
		{"underscore", "kata_satu", `"kata_satu"`, true},
		{"unicode", "pengaturan é", `"pengaturan" "é"`, true},
		{"emoji separator", "a😀b", `"a" "b"`, true},
		{"control", "lo\x00g\tin", `"login"`, true},
		{"phrase inner punct", `"a-b c"`, `"a b c"`, true},
		{"word then phrase", `x"y z"w`, `"x" "y z" "w"`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseQuery(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got %q,%v want %q,%v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestParseQuery_Limits(t *testing.T) {
	var terms []string
	for i := 0; i < 17; i++ {
		terms = append(terms, "t"+string(rune('a'+i)))
	}
	got, _ := ParseQuery(strings.Join(terms, " "))
	if n := strings.Count(got, `"`) / 2; n != MaxQueryTerms {
		t.Fatalf("terms = %d", n)
	}
	got, _ = ParseQuery(strings.Repeat("a", 300))
	if len(got) != MaxQueryRunes+2 {
		t.Fatalf("len = %d", len(got))
	}
}
