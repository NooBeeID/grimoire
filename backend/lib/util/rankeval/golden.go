// Package rankeval menghitung metrik peringkat (recall@k, MRR) untuk golden set pencarian
// dan membandingkannya dengan baseline yang di-commit.
package rankeval

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	KindExactName = "exact-name"
	KindKeyword   = "keyword"
	KindSynonym   = "synonym"
	KindTypo      = "typo"
	KindPhrase    = "phrase"

	MinPairs        = 30
	MinPairsPerKind = 3
	TopK            = 10 // kedalaman peringkat yang dinilai MRR; sama dengan limit tool
)

// Kinds mengembalikan kind valid dalam urutan tetap.
func Kinds() []string {
	return []string{KindExactName, KindKeyword, KindSynonym, KindTypo, KindPhrase}
}

type Pair struct {
	Query  string   `yaml:"query"`
	Expect []string `yaml:"expect"`
	Kind   string   `yaml:"kind"`
}

type Golden struct {
	Pairs []Pair `yaml:"pairs"`
}

// Load membaca YAML golden dari path. Tidak memanggil Validate.
func Load(path string) (Golden, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Golden{}, err
	}
	return Parse(b)
}

// Parse membaca YAML golden dari bytes; kunci tak dikenal gagal keras.
func Parse(b []byte) (Golden, error) {
	var g Golden
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&g); err != nil && !errors.Is(err, io.EOF) {
		return Golden{}, fmt.Errorf("golden: %w", err)
	}
	return g, nil
}

// Validate mengumpulkan semua pelanggaran.
func (g Golden) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if len(g.Pairs) < MinPairs {
		add("hanya %d pasangan (minimal %d)", len(g.Pairs), MinPairs)
	}
	valid := map[string]bool{}
	for _, k := range Kinds() {
		valid[k] = true
	}
	count := map[string]int{}
	seen := map[string]int{}
	for i, p := range g.Pairs {
		q := strings.TrimSpace(p.Query)
		label := fmt.Sprintf("pasangan #%d (query %q)", i+1, p.Query)
		if q == "" {
			add("%s: query kosong", label)
		} else if first, dup := seen[q]; dup {
			add("%s: query duplikat dengan pasangan #%d", label, first)
		} else {
			seen[q] = i + 1
		}
		if !valid[p.Kind] {
			add("%s: kind %q tidak dikenal", label, p.Kind)
		} else {
			count[p.Kind]++
		}
		if len(p.Expect) == 0 {
			add("%s: expect kosong", label)
		}
		exp := map[string]bool{}
		for _, e := range p.Expect {
			switch {
			case strings.TrimSpace(e) == "":
				add("%s: expect berisi string kosong", label)
			case strings.HasPrefix(e, "/"):
				add("%s: expect %q berawalan /", label, e)
			case strings.Contains(e, "\\"):
				add("%s: expect %q memuat backslash", label, e)
			case exp[e]:
				add("%s: expect %q duplikat", label, e)
			}
			exp[e] = true
		}
	}
	for _, k := range Kinds() {
		if count[k] < MinPairsPerKind {
			add("kind %q hanya %d pasang (minimal %d)", k, count[k], MinPairsPerKind)
		}
	}
	return errors.Join(errs...)
}
