package rankeval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const synonymsFixture = "../../../testdata/searcheval/synonyms.yaml"

func TestSynonymsFixture(t *testing.T) {
	groups, err := LoadSynonyms(synonymsFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSynonyms(groups); err != nil {
		t.Fatalf("synonyms.yaml tidak valid: %v", err)
	}
	if len(groups) < 9 {
		t.Errorf("kelompok = %d, ingin >= 9", len(groups))
	}
}

func TestValidateSynonyms(t *testing.T) {
	cases := []struct {
		name   string
		groups [][]string
		want   string
	}{
		{"ok", [][]string{{"a b", "c"}, {"d", "e"}}, ""},
		{"kosong", nil, "kamus kosong"},
		{"satu term", [][]string{{"a"}}, "1 term"},
		{"sebelas term", [][]string{{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}}, "11 term"},
		{"huruf besar", [][]string{{"Login", "masuk"}}, "bukan huruf kecil"},
		{"spasi tepi", [][]string{{" login", "masuk"}}, "spasi tepi"},
		{"term kosong", [][]string{{"login", "  "}}, "term kosong"},
		{"duplikat dalam kelompok", [][]string{{"login", "login"}}, "duplikat"},
		{"duplikat lintas kelompok", [][]string{{"login", "masuk"}, {"masuk", "sign in"}}, "sudah ada di kelompok #1"},
	}
	for _, c := range cases {
		err := ValidateSynonyms(c.groups)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err=%v, ingin memuat %q", c.name, err, c.want)
		}
	}
}

func TestLoadSynonyms_Errors(t *testing.T) {
	if _, err := LoadSynonyms(filepath.Join(t.TempDir(), "tidak-ada.yaml")); err == nil {
		t.Error("file tak ada harus gagal")
	}
	bad := filepath.Join(t.TempDir(), "s.yaml")
	if err := os.WriteFile(bad, []byte("grups: [[a, b]]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSynonyms(bad); err == nil {
		t.Error("kunci tak dikenal harus gagal")
	}
}
