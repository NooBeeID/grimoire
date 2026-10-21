package rankeval

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func validGolden() Golden {
	var g Golden
	for _, k := range Kinds() {
		for i := 0; i < 6; i++ {
			g.Pairs = append(g.Pairs, Pair{Query: fmt.Sprintf("%s q%d", k, i), Expect: []string{"a/b.md"}, Kind: k})
		}
	}
	return g
}

func TestParse(t *testing.T) {
	g, err := Parse([]byte("pairs:\n  - query: \"x\"\n    expect: [\"a.md\"]\n    kind: typo\n"))
	if err != nil || len(g.Pairs) != 1 || g.Pairs[0].Kind != "typo" {
		t.Fatalf("%+v %v", g, err)
	}
	if _, err := Parse([]byte("pairs:\n  - query: x\n    expects: [a]\n    kind: typo\n")); err == nil {
		t.Error("kunci tak dikenal harus error")
	}
	if _, err := Parse([]byte("pairs: [\n")); err == nil {
		t.Error("YAML rusak harus error")
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err=%v", err)
	}
}

func TestValidate(t *testing.T) {
	if err := validGolden().Validate(); err != nil {
		t.Fatalf("golden valid: %v", err)
	}
	mut := func(f func(g *Golden)) error {
		g := validGolden()
		f(&g)
		return g.Validate()
	}
	cases := []struct {
		name string
		f    func(g *Golden)
		want string
	}{
		{"29 pasang", func(g *Golden) { g.Pairs = g.Pairs[:29] }, "minimal 30"},
		{"kind 2 pasang", func(g *Golden) {
			g.Pairs = g.Pairs[:len(g.Pairs)-4] // phrase tinggal 2
			g.Pairs = append(g.Pairs, Pair{Query: "tambahan", Expect: []string{"a"}, Kind: KindKeyword},
				Pair{Query: "tambahan2", Expect: []string{"a"}, Kind: KindKeyword})
		}, `kind "phrase" hanya 2`},
		{"kind tak dikenal", func(g *Golden) { g.Pairs[0].Kind = "aneh" }, `kind "aneh" tidak dikenal`},
		{"expect kosong", func(g *Golden) { g.Pairs[0].Expect = nil }, "expect kosong"},
		{"expect string kosong", func(g *Golden) { g.Pairs[0].Expect = []string{" "} }, "string kosong"},
		{"expect awalan slash", func(g *Golden) { g.Pairs[0].Expect = []string{"/a.md"} }, "berawalan /"},
		{"expect backslash", func(g *Golden) { g.Pairs[0].Expect = []string{`a\b.md`} }, "backslash"},
		{"expect duplikat", func(g *Golden) { g.Pairs[0].Expect = []string{"a", "a"} }, "duplikat"},
		{"query kosong", func(g *Golden) { g.Pairs[0].Query = "   " }, "query kosong"},
		{"query duplikat", func(g *Golden) { g.Pairs[1].Query = " " + g.Pairs[0].Query }, "query duplikat"},
	}
	for _, c := range cases {
		err := mut(c.f)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err=%v want %q", c.name, err, c.want)
		}
	}
}

func TestValidateCollectsAll(t *testing.T) {
	g := validGolden()
	g.Pairs[0].Expect = nil
	g.Pairs[1].Kind = "aneh"
	g.Pairs[2].Query = ""
	err := g.Validate()
	if err == nil {
		t.Fatal("harus error")
	}
	for _, w := range []string{"expect kosong", "tidak dikenal", "query kosong"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("tidak memuat %q: %v", w, err)
		}
	}
}
