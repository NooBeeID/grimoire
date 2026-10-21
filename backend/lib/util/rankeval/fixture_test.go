package rankeval

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

const fixtureDir = "../../../testdata/searcheval"

func TestFixture(t *testing.T) {
	g, err := Load(filepath.Join(fixtureDir, "golden.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("golden tidak valid: %v", err)
	}

	corpus := filepath.Join(fixtureDir, "corpus")
	files := map[string]bool{}
	err = filepath.WalkDir(corpus, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(corpus, p)
		rel = filepath.ToSlash(rel)
		if filepath.Ext(p) != ".md" {
			t.Errorf("%s bukan .md", rel)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(b))) == 0 {
			t.Errorf("%s kosong", rel)
		}
		if !utf8.Valid(b) {
			t.Errorf("%s bukan UTF-8", rel)
		}
		if len(b) > 100*1024 {
			t.Errorf("%s > 100 KB", rel)
		}
		files[rel] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(files); n < 20 || n > 30 {
		t.Errorf("jumlah file korpus %d di luar 20-30", n)
	}

	for i, p := range g.Pairs {
		for _, e := range p.Expect {
			if !files[e] {
				t.Errorf("pasangan #%d (query %q): expect %q tidak ada di corpus/", i+1, p.Query, e)
			}
			if p.Kind == KindSynonym || p.Kind == KindTypo {
				name := strings.TrimSuffix(path.Base(e), ".md")
				if strings.EqualFold(strings.TrimSpace(p.Query), name) {
					t.Errorf("pasangan #%d: query %q sama dengan nama file %q padahal kind %s", i+1, p.Query, name, p.Kind)
				}
			}
		}
	}

	base, err := LoadBaseline(filepath.Join(fixtureDir, "baseline.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		t.Log("baseline.json belum ada; dilewati")
	case err != nil:
		t.Errorf("baseline.json: %v", err)
	case base.Pairs != len(g.Pairs):
		t.Errorf("baseline.Pairs=%d != golden %d; jalankan make search-eval-baseline", base.Pairs, len(g.Pairs))
	}
}
