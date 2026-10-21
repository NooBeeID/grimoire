package base

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestGenerateRawKey_Shape(t *testing.T) {
	rk, err := GenerateRawKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rk.Full) != RawKeyLen || RawKeyLen != 55 {
		t.Fatalf("len=%d", len(rk.Full))
	}
	if rk.Full != "gr_"+rk.Prefix+"_"+rk.Secret {
		t.Fatalf("full=%q", rk.Full)
	}
	p, s, ok := ParseRawKey(rk.Full)
	if !ok || p != rk.Prefix || s != rk.Secret {
		t.Fatalf("parse: %v %q %q", ok, p, s)
	}
}

func TestGenerateRawKey_Deterministic(t *testing.T) {
	src := bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, 10)
	a, err := GenerateRawKey(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateRawKey(bytes.NewReader(src))
	if a != b {
		t.Fatal("not deterministic")
	}
}

func TestGenerateRawKey_RejectionSampling(t *testing.T) {
	// 255 dan 252 harus dilewati; sisanya 0..7 -> "abcdefgh".
	src := append([]byte{255, 252, 0, 1, 2, 3, 4, 5, 6, 7}, make([]byte, SecretBytes)...)
	rk, err := GenerateRawKey(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if rk.Prefix != "abcdefgh" {
		t.Fatalf("prefix=%q", rk.Prefix)
	}
}

func TestGenerateRawKey_ReadError(t *testing.T) {
	if _, err := GenerateRawKey(bytes.NewReader(nil)); !errors.Is(err, io.EOF) || !strings.Contains(err.Error(), "rawkey: read random") {
		t.Fatalf("err=%v", err)
	}
	// prefix lengkap tapi secret kurang
	if _, err := GenerateRawKey(bytes.NewReader(make([]byte, PrefixLen+5))); err == nil {
		t.Fatal("want error")
	}
}

func TestGenerateRawKey_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		rk, err := GenerateRawKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if seen[rk.Full] {
			t.Fatal("duplicate key")
		}
		seen[rk.Full] = true
	}
}

func TestParseRawKey(t *testing.T) {
	rk, _ := GenerateRawKey(bytes.NewReader(append(bytes.Repeat([]byte{3}, PrefixLen), make([]byte, SecretBytes)...)))
	good := rk.Full
	secret := rk.Secret
	// secret dengan bit sisa non-nol: karakter terakhir bukan kelipatan 16 indeks base64.
	nonCanon := good[:len(good)-1] + "B"
	if good[len(good)-1] == 'B' {
		nonCanon = good[:len(good)-1] + "C"
	}
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"valid", good, true},
		{"spaces", "  " + good + "\n", true},
		{"len54", good[:54], false},
		{"len56", good + "A", false},
		{"scheme-upper", "GR" + good[2:], false},
		{"scheme-gx", "gx" + good[2:], false},
		{"prefix-upper", good[:3] + strings.ToUpper(good[3:11]) + good[11:], false},
		{"prefix-dash", good[:3] + "-" + good[4:], false},
		{"no-underscore", good[:11] + "-" + good[12:], false},
		{"secret-plus", good[:12] + "+" + secret[1:], false},
		{"secret-slash", good[:12] + "/" + secret[1:], false},
		{"secret-pad", good[:len(good)-1] + "=", false},
		{"non-canonical", nonCanon, false},
		{"too-long", good + strings.Repeat("a", 202), false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := ParseRawKey(tc.in); ok != tc.ok {
				t.Fatalf("ok=%v want %v for %q", ok, tc.ok, tc.in)
			}
		})
	}
}

func TestParseRawKey_SecretWithUnderscoreDash(t *testing.T) {
	// byte 0xff.. menghasilkan '_' dan '-' di base64url.
	src := append(bytes.Repeat([]byte{0}, PrefixLen), bytes.Repeat([]byte{0xfb, 0xff}, 16)...)
	rk, err := GenerateRawKey(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.ContainsAny(rk.Secret, "_-") {
		t.Fatalf("secret=%q", rk.Secret)
	}
	if _, _, ok := ParseRawKey(rk.Full); !ok {
		t.Fatal("must parse")
	}
}

func TestHashSecret(t *testing.T) {
	h := HashSecret("abc")
	if len(h) != 32 || !bytes.Equal(h, HashSecret("abc")) {
		t.Fatal("hash")
	}
	if bytes.Equal(DummyHash, HashSecret("other")) || len(DummyHash) != 32 {
		t.Fatal("dummy")
	}
}
