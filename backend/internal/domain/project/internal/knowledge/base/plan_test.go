package base

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func plan(t *testing.T, q string, syn [][]string) SearchPlan {
	t.Helper()
	ts, ok := ParseTerms(q)
	if !ok {
		t.Fatalf("query %q tanpa term", q)
	}
	return NewSearchPlan(ts, syn)
}

func altTexts(g QueryGroup, sources ...AltSource) []string {
	var out []string
	for _, a := range g.Alts {
		if len(sources) == 0 || slices.Contains(sources, a.Source) {
			out = append(out, a.Text)
		}
	}
	return out
}

// T20-AC2: sinonim dua arah dalam satu kelompok.
func TestSearchPlan_SynonymBothDirections(t *testing.T) {
	syn := [][]string{{"login", "autentikasi"}}
	for q, want := range map[string][]string{
		"login":       {"login", "autentikasi"},
		"autentikasi": {"autentikasi", "login"},
	} {
		p := plan(t, q, syn)
		if len(p.Groups) != 1 {
			t.Fatalf("%s: groups = %d", q, len(p.Groups))
		}
		if got := altTexts(p.Groups[0], AltOriginal, AltSynonym); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: alts = %v, want %v", q, got, want)
		}
		if !p.Expanded() {
			t.Errorf("%s: Expanded harus true", q)
		}
	}
}

func TestSearchPlan_MultiwordSynonym(t *testing.T) {
	syn := [][]string{{"rollback", "kembalikan versi"}}
	p := plan(t, "kembalikan versi lama", syn)
	if len(p.Groups) != 2 {
		t.Fatalf("groups = %d: %+v", len(p.Groups), p.Groups)
	}
	if got := altTexts(p.Groups[0], AltOriginal, AltSynonym); !reflect.DeepEqual(got, []string{"kembalikan versi", "rollback"}) {
		t.Errorf("g0 = %v", got)
	}
	if got := altTexts(p.Groups[1], AltOriginal, AltSynonym); !reflect.DeepEqual(got, []string{"lama"}) {
		t.Errorf("g1 = %v", got)
	}

	p = plan(t, `"kembalikan versi"`, syn)
	if len(p.Groups) != 1 || !slices.Contains(altTexts(p.Groups[0]), "rollback") {
		t.Fatalf("quoted = %+v", p.Groups)
	}
	// Frasa berkutip hanya cocok bila seluruh katanya = satu term kamus.
	p = plan(t, `"kembalikan versi lama"`, syn)
	if p.Expanded() {
		t.Fatalf("frasa lebih panjang tidak boleh cocok: %+v", p.Groups)
	}
}

func TestSearchPlan_HyphenTerm(t *testing.T) {
	p := plan(t, "rate limit", [][]string{{"rate-limit", "batas permintaan"}})
	if len(p.Groups) != 1 || !slices.Contains(altTexts(p.Groups[0], AltSynonym), "batas permintaan") {
		t.Fatalf("groups = %+v", p.Groups)
	}
}

func TestSearchPlan_NoSynonyms(t *testing.T) {
	p := plan(t, "kirim", nil)
	if p.Expanded() {
		t.Fatal("tanpa sinonim/typo tidak boleh Expanded")
	}
	pp := p.Params()
	stemOnly := map[string]bool{}
	for i, txt := range pp.Texts {
		stemOnly[txt] = pp.StemOnly[i]
	}
	if v, ok := stemOnly["mengirim"]; !ok || !v {
		t.Fatalf("varian mengirim harus ada dan StemOnly: %v", stemOnly)
	}
	if stemOnly["kirim"] {
		t.Fatal("alternatif asli tidak StemOnly")
	}
}

func TestSearchPlan_SeparateWordsAreSeparateGroups(t *testing.T) {
	p := plan(t, "reset password", nil)
	if len(p.Groups) != 2 || p.Groups[0].Alts[0].Text != "reset" || p.Groups[1].Alts[0].Text != "password" {
		t.Fatalf("groups = %+v", p.Groups)
	}
	pp := p.Params()
	if len(pp.Texts) != len(pp.Groups) || len(pp.Texts) != len(pp.StemOnly) {
		t.Fatal("kolom paralel harus sama panjang")
	}
	if pp.Groups[0] != 0 || pp.Groups[len(pp.Groups)-1] != 1 {
		t.Fatalf("groups = %v", pp.Groups)
	}
}

func TestSearchPlan_TypoCandidatesBounds(t *testing.T) {
	long := strings.Repeat("a", 65)
	p := plan(t, "rollbak api 2026 "+long+" login", [][]string{{"login", "masuk"}})
	got := p.TypoCandidates()
	if !reflect.DeepEqual(got, []string{"rollbak"}) {
		t.Fatalf("candidates = %v", got)
	}
	if p.Expanded() != true { // login punya sinonim
		t.Fatal("login punya sinonim -> Expanded")
	}

	q := plan(t, "rollbak", nil)
	if q.Expanded() {
		t.Fatal("belum ada typo")
	}
	w := q.WithTypos(map[string][]string{"rollbak": {"rollback"}})
	if !w.Expanded() || !slices.Contains(altTexts(w.Groups[0], AltTypo), "rollback") {
		t.Fatalf("with typos = %+v", w.Groups)
	}
	if q.Expanded() || len(q.Groups[0].Alts) != 1+len(altTexts(q.Groups[0], AltVariant)) {
		t.Fatal("receiver tidak boleh berubah")
	}
}

func TestSearchPlan_TypoCandidates_PhraseAndDigitsExcluded(t *testing.T) {
	p := plan(t, `"exponential backoff" 12345`, nil)
	if got := p.TypoCandidates(); len(got) != 0 {
		t.Fatalf("candidates = %v", got)
	}
}

func TestSearchPlan_WithTypos_MaxCandidates(t *testing.T) {
	p := plan(t, "rollbak", nil).WithTypos(map[string][]string{"rollbak": {"a1a1", "b2b2", "c3c3", "d4d4"}})
	if n := len(altTexts(p.Groups[0], AltTypo)); n != TypoMaxCandidates {
		t.Fatalf("typo alts = %d", n)
	}
}

func TestSearchPlan_Caps(t *testing.T) {
	var syn [][]string
	var words []string
	for i := 0; i < MaxQueryTerms; i++ {
		w := fmt.Sprintf("w%dx", i)
		words = append(words, w)
		g := []string{w}
		for j := 0; j < 9; j++ {
			g = append(g, fmt.Sprintf("s%d%dy", i, j))
		}
		syn = append(syn, g)
	}
	p := plan(t, strings.Join(words, " "), syn)
	pp := p.Params()
	if len(pp.Texts) > MaxPlanAlts {
		t.Fatalf("alts = %d > %d", len(pp.Texts), MaxPlanAlts)
	}
	for i, g := range p.Groups {
		if g.Alts[0].Source != AltOriginal || g.Alts[0].Text != words[i] {
			t.Fatalf("alternatif asli grup %d hilang: %+v", i, g.Alts)
		}
		if len(g.Alts) > MaxAltsPerGroup {
			t.Fatalf("grup %d: %d alt", i, len(g.Alts))
		}
	}
}

func TestSearchPlan_SynonymsInsertedFromDictionaryAreNormalized(t *testing.T) {
	p := plan(t, "LOGIN", [][]string{{" Login ", "Masuk  Aplikasi"}})
	if got := altTexts(p.Groups[0], AltSynonym); !reflect.DeepEqual(got, []string{"masuk aplikasi"}) {
		t.Fatalf("synonyms = %v", got)
	}
}
