package base

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTerms(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
		r    Reason
	}{
		{"trim dan huruf kecil", []string{" Login ", "Autentikasi"}, []string{"login", "autentikasi"}, ""},
		{"spasi diringkas", []string{"Rate  Limit", "batas permintaan"}, []string{"rate limit", "batas permintaan"}, ""},
		{"hyphen dan underscore", []string{"rate-limit", "rate_limit"}, []string{"rate-limit", "rate_limit"}, ""},
		{"terlalu sedikit", []string{"a"}, nil, ReasonTooFewTerms},
		{"kosong", nil, nil, ReasonTooFewTerms},
		{"terlalu banyak", strings.Fields("a1 a2 a3 a4 a5 a6 a7 a8 a9 a10 a11"), nil, ReasonTooManyTerms},
		{"duplikat setelah normalisasi", []string{"login", "LOGIN"}, nil, ReasonDuplicateTerm},
		{"term kosong", []string{"", "x"}, nil, ReasonTermEmpty},
		{"term spasi saja", []string{"   ", "x"}, nil, ReasonTermEmpty},
		{"terlalu panjang", []string{strings.Repeat("a", 65), "x"}, nil, ReasonTermTooLong},
		{"64 rune boleh", []string{strings.Repeat("é", 64), "x"}, []string{strings.Repeat("é", 64), "x"}, ""},
		{"karakter ampersand", []string{"a&b", "c"}, nil, ReasonTermInvalidChar},
		{"karakter operator", []string{"x:*", "y"}, nil, ReasonTermInvalidChar},
		{"tanda kutip", []string{`"x"`, "y"}, nil, ReasonTermInvalidChar},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, r := NormalizeTerms(c.in)
			if r != c.r {
				t.Fatalf("reason = %q, want %q", r, c.r)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("terms = %v, want %v", got, c.want)
			}
		})
	}
}

func TestMaxTermsBoundary(t *testing.T) {
	ten := strings.Fields("a1 a2 a3 a4 a5 a6 a7 a8 a9 a10")
	if got, r := NormalizeTerms(ten); r != "" || len(got) != 10 {
		t.Fatalf("10 term harus valid: %v %q", got, r)
	}
}

func TestGroupLifecycle(t *testing.T) {
	a := Actor{ID: "u1", Label: "Budi", Role: "developer"}
	g := NewGroup("p1", []string{"login", "masuk"}, a)
	if g.ProjectID != "p1" || g.CreatedBy != "u1" || g.UpdatedByLabel != "Budi" {
		t.Fatalf("group = %+v", g)
	}
	b := Actor{ID: "u2", Label: "Sari"}
	g.CreatedAt = time.Unix(1, 0)
	g2 := g.WithTerms([]string{"a1", "b1"}, b)
	if g2.CreatedBy != "u1" || g2.UpdatedBy != "u2" || g2.UpdatedByLabel != "Sari" || g2.Terms[0] != "a1" {
		t.Fatalf("g2 = %+v", g2)
	}
	if g.Terms[0] != "login" {
		t.Fatal("WithTerms tidak boleh mengubah receiver")
	}
	if d := (Group{}).ToDTO(); d.Terms == nil {
		t.Fatal("DTO Terms tidak boleh nil")
	}
}
