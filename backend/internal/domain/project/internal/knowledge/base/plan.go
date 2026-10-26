package base

import (
	"strings"
	"unicode"

	"grimoire/lib/util/stemid"
)

// AltSource menandai asal sebuah alternatif query.
type AltSource uint8

const (
	AltOriginal AltSource = iota // kata/frasa yang diketik user
	AltSynonym                   // dari kamus sinonim project
	AltTypo                      // kandidat koreksi typo (trigram)
	AltVariant                   // varian peluluhan K/P/T/S (hanya untuk indeks stem)
)

// Alt adalah satu alternatif; Text = kata dipisah satu spasi.
type Alt struct {
	Text   string
	Source AltSource
}

// QueryGroup adalah satu term user beserta alternatifnya; Alts[0] selalu asli.
type QueryGroup struct{ Alts []Alt }

// SearchPlan adalah query terurai yang dikirim ke SQL hanya sebagai array parameter.
type SearchPlan struct{ Groups []QueryGroup }

// normalizeDictTerm menormalkan term kamus sama seperti query: huruf kecil, dipecah pada non-rune-kata
// sehingga `rate-limit` == `rate limit`.
func normalizeDictTerm(s string) ([]string, string) {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !isWordRune(r) })
	return words, strings.Join(words, " ")
}

type dictionary struct {
	groups   [][]string       // term ternormalisasi per kelompok
	byPhrase map[string][]int // frasa -> indeks kelompok
	maxWords int
}

func newDictionary(synonyms [][]string) dictionary {
	d := dictionary{byPhrase: map[string][]int{}}
	for _, g := range synonyms {
		var terms []string
		seen := map[string]bool{}
		for _, raw := range g {
			words, text := normalizeDictTerm(raw)
			if text == "" || seen[text] {
				continue
			}
			seen[text] = true
			terms = append(terms, text)
			if len(words) > d.maxWords {
				d.maxWords = len(words)
			}
		}
		if len(terms) < 2 {
			continue
		}
		idx := len(d.groups)
		d.groups = append(d.groups, terms)
		for _, t := range terms {
			d.byPhrase[t] = append(d.byPhrase[t], idx)
		}
	}
	return d
}

func (d dictionary) group(phrase string) QueryGroup {
	g := QueryGroup{Alts: []Alt{{Text: phrase, Source: AltOriginal}}}
	for _, gi := range d.byPhrase[phrase] {
		for _, t := range d.groups[gi] {
			g.Alts = appendAlt(g.Alts, Alt{Text: t, Source: AltSynonym})
		}
	}
	return g
}

func appendAlt(alts []Alt, a Alt) []Alt {
	for _, x := range alts {
		if x.Text == a.Text {
			return alts
		}
	}
	return append(alts, a)
}

func singleWord(phrase string) QueryGroup {
	g := QueryGroup{Alts: []Alt{{Text: phrase, Source: AltOriginal}}}
	return g
}

func withVariants(g QueryGroup) QueryGroup {
	if strings.Contains(g.Alts[0].Text, " ") {
		return g
	}
	for _, v := range stemid.Variants(g.Alts[0].Text) {
		g.Alts = appendAlt(g.Alts, Alt{Text: v, Source: AltVariant})
	}
	return g
}

// NewSearchPlan menyusun rencana dari term user dan kelompok sinonim project (dua arah).
// Deret kata tak berkutip dicocokkan serakah kiri-ke-kanan dengan kecocokan terpanjang dulu; term
// berkutip cocok hanya bila seluruh katanya sama dengan satu term kamus.
func NewSearchPlan(terms []Term, synonyms [][]string) SearchPlan {
	dict := newDictionary(synonyms)
	var plan SearchPlan
	addGroup := func(g QueryGroup) { plan.Groups = append(plan.Groups, withVariants(g)) }

	var run []string // kata tak berkutip berurutan
	flushRun := func() {
		for i := 0; i < len(run); {
			matched := 0
			maxL := min(dict.maxWords, len(run)-i)
			for l := maxL; l >= 1; l-- {
				if _, ok := dict.byPhrase[strings.Join(run[i:i+l], " ")]; ok {
					matched = l
					break
				}
			}
			if matched == 0 {
				addGroup(singleWord(run[i]))
				i++
				continue
			}
			addGroup(dict.group(strings.Join(run[i:i+matched], " ")))
			i += matched
		}
		run = run[:0]
	}
	for _, t := range terms {
		words := cleanWords(t.Words)
		if len(words) == 0 {
			continue
		}
		if !t.Quoted {
			run = append(run, words...)
			continue
		}
		flushRun()
		phrase := strings.Join(words, " ")
		if _, ok := dict.byPhrase[phrase]; ok {
			addGroup(dict.group(phrase))
		} else {
			addGroup(singleWord(phrase))
		}
	}
	flushRun()
	plan.capAlts()
	return plan
}

// cleanWords menjaga invarian rencana: huruf kecil dan hanya rune kata, apa pun sumber Term.
func cleanWords(in []string) []string {
	var out []string
	for _, w := range in {
		for _, p := range strings.FieldsFunc(strings.ToLower(w), func(r rune) bool { return !isWordRune(r) }) {
			out = append(out, p)
		}
	}
	return out
}

// capAlts memotong alternatif: maks MaxAltsPerGroup per grup, total maks MaxPlanAlts. Alternatif asli
// (indeks 0) tidak pernah dibuang; yang dipotong lebih dulu adalah ekor (varian/typo).
func (p *SearchPlan) capAlts() {
	total := 0
	for i := range p.Groups {
		if len(p.Groups[i].Alts) > MaxAltsPerGroup {
			p.Groups[i].Alts = p.Groups[i].Alts[:MaxAltsPerGroup]
		}
		total += len(p.Groups[i].Alts)
	}
	for total > MaxPlanAlts {
		best := -1
		for i := range p.Groups {
			if len(p.Groups[i].Alts) > 1 && (best < 0 || len(p.Groups[i].Alts) >= len(p.Groups[best].Alts)) {
				best = i
			}
		}
		if best < 0 {
			return
		}
		p.Groups[best].Alts = p.Groups[best].Alts[:len(p.Groups[best].Alts)-1]
		total--
	}
}

func allDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

// TypoCandidates mengembalikan kata yang layak dicek typo: grup satu-kata tanpa sinonim, panjang
// TypoMinRunes..TypoMaxRunes, bukan angka saja, unik, urut kemunculan.
func (p SearchPlan) TypoCandidates() []string {
	var out []string
	seen := map[string]bool{}
	for _, g := range p.Groups {
		w := g.Alts[0].Text
		if strings.Contains(w, " ") || seen[w] {
			continue
		}
		plain := true
		for _, a := range g.Alts {
			if a.Source == AltSynonym || a.Source == AltTypo {
				plain = false
				break
			}
		}
		n := len([]rune(w))
		if !plain || n < TypoMinRunes || n > TypoMaxRunes || allDigits(w) {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// WithTypos mengembalikan salinan rencana dengan kandidat koreksi (kata -> saran) sebagai AltTypo,
// paling banyak TypoMaxCandidates per kata. Receiver tidak berubah.
func (p SearchPlan) WithTypos(s map[string][]string) SearchPlan {
	out := SearchPlan{Groups: make([]QueryGroup, len(p.Groups))}
	for i, g := range p.Groups {
		out.Groups[i] = QueryGroup{Alts: append([]Alt(nil), g.Alts...)}
		sugg, ok := s[g.Alts[0].Text]
		if !ok || strings.Contains(g.Alts[0].Text, " ") {
			continue
		}
		added := 0
		for _, sg := range sugg {
			if added >= TypoMaxCandidates {
				break
			}
			words := cleanWords([]string{sg})
			if len(words) != 1 {
				continue
			}
			before := len(out.Groups[i].Alts)
			out.Groups[i].Alts = appendAlt(out.Groups[i].Alts, Alt{Text: words[0], Source: AltTypo})
			if len(out.Groups[i].Alts) > before {
				added++
			}
		}
	}
	out.capAlts()
	return out
}

// Expanded true bila ada alternatif sinonim atau typo (varian tidak dihitung).
func (p SearchPlan) Expanded() bool {
	for _, g := range p.Groups {
		for _, a := range g.Alts {
			if a.Source == AltSynonym || a.Source == AltTypo {
				return true
			}
		}
	}
	return false
}

// PlanParams adalah bentuk kolom paralel untuk unnest di SQL: satu baris per alternatif.
type PlanParams struct {
	Texts    []string
	Groups   []int32
	StemOnly []bool
}

func (p SearchPlan) Params() PlanParams {
	var pp PlanParams
	for gi, g := range p.Groups {
		for _, a := range g.Alts {
			pp.Texts = append(pp.Texts, a.Text)
			pp.Groups = append(pp.Groups, int32(gi))
			pp.StemOnly = append(pp.StemOnly, a.Source == AltVariant)
		}
	}
	return pp
}
