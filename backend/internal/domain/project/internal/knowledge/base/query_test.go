package base

import (
	"strings"
	"testing"
)

func TestParseTerms_OperatorsInert(t *testing.T) {
	ts, ok := ParseTerms("a & b | !c:* <-> 'd'")
	if !ok || strings.Join(flat(ts), ",") != "a,b,c,d" {
		t.Fatalf("terms = %v ok=%v", flat(ts), ok)
	}
	for _, in := range []string{"-", "!!", "", "   ", `"""`, "&|:*", "''"} {
		if ts, ok := ParseTerms(in); ok || ts != nil {
			t.Errorf("ParseTerms(%q) = %v,%v; want nil,false", in, ts, ok)
		}
	}
	ts, ok = ParseTerms(`"unclosed phrase`)
	if !ok || len(ts) != 1 || !ts[0].Quoted || strings.Join(ts[0].Words, " ") != "unclosed phrase" {
		t.Fatalf("kutip tak tertutup = %v", flat(ts))
	}
}

func TestParseTerms_Lowercase(t *testing.T) {
	ts, _ := ParseTerms(`Reset "Rate Limit"`)
	if got := strings.Join(flat(ts), ","); got != `reset,"rate limit"` {
		t.Fatalf("got %s", got)
	}
}

func TestParseTerms_Long(t *testing.T) {
	var parts []string
	for i := 0; i < 40; i++ {
		parts = append(parts, "kata"+string(rune('a'+i%26)))
	}
	ts, ok := ParseTerms(strings.Join(parts, " "))
	if !ok || len(ts) > MaxQueryTerms {
		t.Fatalf("len = %d", len(ts))
	}
	ts, _ = ParseTerms(strings.Repeat("a", 300))
	if len(ts) != 1 || len([]rune(ts[0].Words[0])) != MaxQueryRunes {
		t.Fatalf("panjang kata = %d", len([]rune(ts[0].Words[0])))
	}
}

func flat(ts []Term) []string {
	var out []string
	for _, t := range ts {
		w := strings.Join(t.Words, " ")
		if t.Quoted {
			w = `"` + w + `"`
		}
		out = append(out, w)
	}
	return out
}
