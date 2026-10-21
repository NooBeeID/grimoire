package rankeval

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleMetrics() Metrics {
	return Metrics{Pairs: 5, Recall1: 0.123456, Recall5: 0.5, MRR: 0.3333333,
		PerKind: map[string]KindMetrics{
			KindPhrase:  {Pairs: 2, Recall1: 1, Recall5: 1, MRR: 1},
			KindKeyword: {Pairs: 3, Recall1: 0, Recall5: 0.25, MRR: 0.1},
		}}
}

func TestBaselineRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "b.json")
	if err := SaveBaseline(p, sampleMetrics()); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(p)
	if err := SaveBaseline(p, sampleMetrics()); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(p)
	if string(first) != string(second) {
		t.Error("tidak deterministik")
	}
	s := string(first)
	if !strings.HasSuffix(s, "\n") {
		t.Error("harus berakhir newline")
	}
	if !strings.Contains(s, "0.1235") || strings.Contains(s, "0.123456") {
		t.Errorf("pembulatan: %s", s)
	}
	if strings.Index(s, `"keyword"`) > strings.Index(s, `"phrase"`) {
		t.Error("urutan per_kind harus mengikuti Kinds()")
	}
	m, err := LoadBaseline(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Pairs != 5 || !near(m.Recall1, 0.1235) || !near(m.MRR, 0.3333) || m.PerKind[KindKeyword].Pairs != 3 {
		t.Errorf("%+v", m)
	}
}

func TestLoadBaselineErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadBaseline(filepath.Join(dir, "no.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err=%v", err)
	}
	for name, body := range map[string]string{
		"asing.json": `{"pairs":1,"foo":2}`,
		"nol.json":   `{"pairs":0}`,
		"rusak.json": `{`,
	} {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte(body), 0o644)
		if _, err := LoadBaseline(p); err == nil {
			t.Errorf("%s harus error", name)
		}
	}
}
