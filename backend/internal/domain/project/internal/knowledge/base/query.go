package base

import (
	"strings"
	"unicode"
)

const (
	MaxQueryRunes = 200
	MaxQueryTerms = 16
)

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// Term adalah satu unit query: kata tunggal, atau frasa berkutip (Quoted). Words huruf kecil dan hanya
// berisi rune kata (isWordRune); operator tsquery/trigram tidak pernah lolos ke sini.
type Term struct {
	Words  []string
	Quoted bool
}

// tokenize membersihkan q (kontrol dibuang, dipotong MaxQueryRunes) lalu memecahnya menjadi term
// (maks MaxQueryTerms). Huruf besar dipertahankan; pemanggil yang menormalkan.
func tokenize(q string) []Term {
	q = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, q)
	q = strings.TrimSpace(q)
	if r := []rune(q); len(r) > MaxQueryRunes {
		q = string(r[:MaxQueryRunes])
	}

	var terms []Term
	add := func(words []string, quoted bool) {
		if len(words) > 0 && len(terms) < MaxQueryTerms {
			terms = append(terms, Term{Words: words, Quoted: quoted})
		}
	}
	var word, phrase strings.Builder
	inQuote := false
	flushWord := func() {
		if word.Len() > 0 {
			add([]string{word.String()}, false)
			word.Reset()
		}
	}
	flushPhrase := func() {
		add(strings.FieldsFunc(phrase.String(), func(r rune) bool { return !isWordRune(r) }), true)
		phrase.Reset()
	}
	for _, r := range q {
		switch {
		case inQuote && r == '"':
			flushPhrase()
			inQuote = false
		case inQuote:
			phrase.WriteRune(r)
		case r == '"':
			flushWord()
			inQuote = true
		case isWordRune(r):
			word.WriteRune(r)
		default:
			flushWord()
		}
	}
	flushWord()
	if inQuote {
		flushPhrase()
	}
	return terms
}

// ParseTerms menguraikan kueri user menjadi term huruf kecil. Tanpa term -> (nil, false); caller
// mengembalikan hasil kosong tanpa query DB. Batas MaxQueryRunes dan MaxQueryTerms berlaku.
func ParseTerms(q string) ([]Term, bool) {
	terms := tokenize(q)
	if len(terms) == 0 {
		return nil, false
	}
	for i, t := range terms {
		for j, w := range t.Words {
			terms[i].Words[j] = strings.ToLower(w)
		}
	}
	return terms, true
}
