package listendpoints

import (
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/lib/contracts/project/apispec/listendpoints"
)

func TestNormalize_LimitClamp(t *testing.T) {
	in := []int{-5, 0, 1, 100, 300, 301, 100000}
	want := []int{100, 100, 1, 100, 300, 300, 300}
	for i, l := range in {
		if got := normalize(listendpoints.Request{Limit: l}).Limit; got != want[i] {
			t.Errorf("limit %d: got %d want %d", l, got, want[i])
		}
	}
}

func TestNormalize_OffsetClamp(t *testing.T) {
	in := []int{-1, 0, 42}
	want := []int{0, 0, 42}
	for i, o := range in {
		if got := normalize(listendpoints.Request{Offset: o}).Offset; got != want[i] {
			t.Errorf("offset %d: got %d", o, got)
		}
	}
}

func TestNormalize_TrimsAndEmpties(t *testing.T) {
	f := normalize(listendpoints.Request{Spec: "  x  ", Tag: " y ", Query: "  z "})
	if f.Spec != "x" || f.Tag != "y" || f.Pattern != "%z%" {
		t.Fatalf("%+v", f)
	}
	f = normalize(listendpoints.Request{Spec: "   ", Tag: "   ", Query: "   "})
	if f.Spec != "" || f.Tag != "" || f.Pattern != "" {
		t.Fatalf("%+v", f)
	}
}

func TestNormalize_QueryTruncatedToRunes(t *testing.T) {
	f := normalize(listendpoints.Request{Query: strings.Repeat("é", 250)})
	inner := strings.TrimSuffix(strings.TrimPrefix(f.Pattern, "%"), "%")
	if utf8.RuneCountInString(inner) != listendpoints.MaxQueryRunes || !utf8.ValidString(inner) {
		t.Fatalf("runes=%d", utf8.RuneCountInString(inner))
	}
}

func TestNormalize_StripsNULAndInvalidUTF8(t *testing.T) {
	f := normalize(listendpoints.Request{Query: "a\x00b\xffc"})
	if f.Pattern != "%abc%" {
		t.Fatalf("%q", f.Pattern)
	}
}

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		`%`: `\%`, `_`: `\_`, `\`: `\\`, `100%_x\y`: `100\%\_x\\y`, `plain`: `plain`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestNormalize_PatternWrapped(t *testing.T) {
	if got := normalize(listendpoints.Request{Query: "a_b"}).Pattern; got != `%a\_b%` {
		t.Fatalf("%q", got)
	}
}

func TestValidProjectID(t *testing.T) {
	const ok = "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10"
	cases := map[string]bool{
		ok: true, "": false, "bukan-uuid": false, "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e1": false,
		"urn:uuid:" + ok: false, "{" + ok + "}": false, ok + " ": false, "0b6c2e1a7d4f4c1e9a553f2b8c9d1e10": false,
	}
	for in, want := range cases {
		if got := validProjectID(in); got != want {
			t.Errorf("%q: got %v", in, got)
		}
	}
}
