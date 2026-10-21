package base

import (
	"strings"
	"testing"
)

func TestPrepareContent(t *testing.T) {
	bom := "\xEF\xBB\xBF"
	cases := []struct {
		name string
		in   string
		max  int64
		want string
		r    Reason
	}{
		{"plain", "halo", 10, "halo", ""},
		{"bom stripped", bom + "halo", 10, "halo", ""},
		{"exact max", strings.Repeat("a", 10), 10, strings.Repeat("a", 10), ""},
		{"over max", strings.Repeat("a", 11), 10, "", ReasonTooLarge},
		{"bom plus exact max", bom + strings.Repeat("a", 10), 10, strings.Repeat("a", 10), ""},
		{"nul", "a\x00b", 10, "", ReasonInvalidEncoding},
		{"invalid utf8", "\xff\xfe", 10, "", ReasonInvalidEncoding},
		{"png header", "\x89PNG\r\n\x1a\n\x00", 100, "", ReasonInvalidEncoding},
		{"empty", "", 10, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, r := PrepareContent([]byte(tc.in), tc.max)
			if got != tc.want || r != tc.r {
				t.Fatalf("got %q,%q want %q,%q", got, r, tc.want, tc.r)
			}
		})
	}
}

func TestHashVersion(t *testing.T) {
	h := Hash("abc")
	if len(h) != 32 {
		t.Fatal("hash len")
	}
	v := Version(h)
	if v != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" || !ValidVersion(v) {
		t.Fatalf("version = %s", v)
	}
}

func TestValidVersion(t *testing.T) {
	ok := strings.Repeat("a", 64)
	for v, want := range map[string]bool{
		ok: true, strings.Repeat("A", 64): false, strings.Repeat("a", 63): false,
		strings.Repeat("g", 64): false, "": false, strings.Repeat("0", 65): false,
	} {
		if ValidVersion(v) != want {
			t.Errorf("ValidVersion(%q) != %v", v, want)
		}
	}
}

func TestTitle(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"basic", "# Login\n", "Login"},
		{"only h2", "## Sub\n", ""},
		{"no space", "#NoSpace\n", ""},
		{"closing hash", "# Judul #\n", "Judul"},
		{"hash in word", "# C#\n", "C#"},
		{"fenced ignored", "```\n# Not title\n```\n# Real\n", "Real"},
		{"after 4KiB", strings.Repeat("x", 4100) + "\n# Late\n", ""},
		{"crlf", "intro\r\n# Judul\r\n", "Judul"},
		{"first wins", "# A\n# B\n", "A"},
		{"long", "# " + strings.Repeat("é", 300) + "\n", strings.Repeat("é", 255)},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Title(tc.in); got != tc.want {
				t.Fatalf("Title = %q, want %q", got, tc.want)
			}
		})
	}
}
