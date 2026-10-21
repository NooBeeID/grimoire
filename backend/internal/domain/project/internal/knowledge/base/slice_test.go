package base

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSliceUTF8_Basic(t *testing.T) {
	part, s, e, tr, r := SliceUTF8("abcdef", 1, 3)
	if part != "bcd" || s != 1 || e != 4 || !tr || r != "" {
		t.Fatalf("%q %d %d %v %q", part, s, e, tr, r)
	}
	part, _, e, tr, _ = SliceUTF8("abc", 0, 0)
	if part != "abc" || e != 3 || tr {
		t.Fatalf("%q %d %v", part, e, tr)
	}
}

func TestSliceUTF8_MultibyteBoundaries(t *testing.T) {
	// "é" = 2 byte, "😀" = 4 byte
	s := "aé😀b"
	part, st, e, _, _ := SliceUTF8(s, 2, 100) // offset di tengah é -> maju ke 3
	if st != 3 || part != "😀b" || e != len(s) {
		t.Fatalf("%q %d %d", part, st, e)
	}
	part, st, e, tr, _ := SliceUTF8(s, 0, 4) // end=4 di tengah 😀 -> mundur ke 3
	if part != "aé" || st != 0 || e != 3 || !tr {
		t.Fatalf("%q %d %d %v", part, st, e, tr)
	}
	part, st, e, tr, _ = SliceUTF8(s, 3, 1) // limit < rune -> tetap maju satu rune
	if part != "😀" || st != 3 || e != 7 || !tr {
		t.Fatalf("%q %d %d %v", part, st, e, tr)
	}
}

func TestSliceUTF8_Offsets(t *testing.T) {
	if _, _, _, _, r := SliceUTF8("abc", -1, 1); r != ReasonInvalidOffset {
		t.Fatal("negative")
	}
	if _, _, _, _, r := SliceUTF8("abc", 4, 1); r != ReasonInvalidOffset {
		t.Fatal("len+1")
	}
	part, st, e, tr, r := SliceUTF8("abc", 3, 1)
	if part != "" || st != 3 || e != 3 || tr || r != "" {
		t.Fatalf("%q %d %d %v %q", part, st, e, tr, r)
	}
	// offset di tengah rune terakhir
	part, st, _, tr, _ = SliceUTF8("a😀", 3, 10)
	if part != "" || st != 5 || tr {
		t.Fatalf("%q %d %v", part, st, tr)
	}
}

func TestSliceUTF8_ChainReassembles(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []rune("ab é😀中\n")
	for iter := 0; iter < 200; iter++ {
		var sb strings.Builder
		for i, n := 0, rng.Intn(60); i < n; i++ {
			sb.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		content := sb.String()
		limit := 1 + rng.Intn(7)
		var got strings.Builder
		off := 0
		for steps := 0; ; steps++ {
			if steps > len(content)+2 {
				t.Fatalf("no progress on %q limit %d", content, limit)
			}
			part, _, end, tr, r := SliceUTF8(content, off, limit)
			if r != "" || !utf8.ValidString(part) {
				t.Fatalf("bad slice %q %q", part, r)
			}
			got.WriteString(part)
			off = end
			if !tr {
				break
			}
		}
		if got.String() != content {
			t.Fatalf("reassembled %q != %q (limit %d)", got.String(), content, limit)
		}
	}
}
