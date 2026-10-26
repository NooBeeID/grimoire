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
	minSynonymTerms = 2
	maxSynonymTerms = 10
)

type synonymFile struct {
	Groups [][]string `yaml:"groups"`
}

// LoadSynonyms membaca kamus sinonim fixture (`groups: [[term, ...], ...]`). Kunci tak dikenal gagal keras.
// Tidak memanggil ValidateSynonyms.
func LoadSynonyms(path string) ([][]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f synonymFile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("synonyms: %w", err)
	}
	return f.Groups, nil
}

// ValidateSynonyms memastikan tiap kelompok berisi 2-10 term huruf kecil tanpa spasi tepi,
// unik dalam kelompok dan lintas kelompok. Semua pelanggaran dikumpulkan.
func ValidateSynonyms(groups [][]string) error {
	var errs []error
	if len(groups) == 0 {
		errs = append(errs, errors.New("kamus kosong"))
	}
	owner := map[string]int{}
	for i, g := range groups {
		label := fmt.Sprintf("kelompok #%d", i+1)
		if len(g) < minSynonymTerms || len(g) > maxSynonymTerms {
			errs = append(errs, fmt.Errorf("%s: %d term (harus %d-%d)", label, len(g), minSynonymTerms, maxSynonymTerms))
		}
		for _, t := range g {
			switch {
			case strings.TrimSpace(t) == "":
				errs = append(errs, fmt.Errorf("%s: term kosong", label))
				continue
			case t != strings.TrimSpace(t):
				errs = append(errs, fmt.Errorf("%s: term %q punya spasi tepi", label, t))
			case t != strings.ToLower(t):
				errs = append(errs, fmt.Errorf("%s: term %q bukan huruf kecil", label, t))
			}
			if prev, ok := owner[t]; ok {
				if prev == i {
					errs = append(errs, fmt.Errorf("%s: term %q duplikat", label, t))
				} else {
					errs = append(errs, fmt.Errorf("%s: term %q sudah ada di kelompok #%d", label, t, prev+1))
				}
				continue
			}
			owner[t] = i
		}
	}
	return errors.Join(errs...)
}
