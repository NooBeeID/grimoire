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

// ParseQuery membersihkan kueri user menjadi masukan aman untuk websearch_to_tsquery('simple', $q).
// Kata dan frasa ("...") menjadi term berkutip, sehingga or, -, &, :, ! tidak pernah menjadi operator.
// Tanpa term -> ("", false); caller mengembalikan hasil kosong tanpa query DB.
func ParseQuery(q string) (string, bool) {
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

	var terms []string
	add := func(words []string) {
		if len(words) > 0 && len(terms) < MaxQueryTerms {
			terms = append(terms, strings.Join(words, " "))
		}
	}
	var word, phrase strings.Builder
	inQuote := false
	flushWord := func() {
		if word.Len() > 0 {
			add([]string{word.String()})
			word.Reset()
		}
	}
	flushPhrase := func() {
		add(strings.FieldsFunc(phrase.String(), func(r rune) bool { return !isWordRune(r) }))
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
	if len(terms) == 0 {
		return "", false
	}
	for i, t := range terms {
		terms[i] = `"` + t + `"`
	}
	return strings.Join(terms, " "), true
}
