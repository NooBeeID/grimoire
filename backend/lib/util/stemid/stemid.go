// Package stemid menghasilkan varian peluluhan awalan Indonesia (K/P/T/S) untuk sisi query (T20 K3).
// Snowball `indonesian` Postgres tidak memulihkan huruf awal yang luluh (meng+k, mem+p, men+t, meny+s),
// sehingga `kirim` dan `mengirim` menjadi lexeme berbeda. Fungsi murni ini menutup celah itu; hasilnya
// hanya dipakai sebagai alternatif query stem. Hanya stdlib (rules 4.9).
package stemid

import "strings"

const minRunes = 4

const vowels = "aiueo"

func isVowelAt(s string, i int) bool {
	return i < len(s) && strings.IndexByte(vowels, s[i]) >= 0
}

// peluluhan: awalan bentuk luruh -> huruf awal yang luluh. Urutan penting (meny/peny sebelum men/pen).
var prefixes = []struct{ prefix, lead string }{
	{"meng", "k"}, {"peng", "k"},
	{"meny", "s"}, {"peny", "s"},
	{"mem", "p"}, {"pem", "p"},
	{"men", "t"}, {"pen", "t"},
}

// reverse: huruf awal kata dasar -> pasangan awalan (verba, nomina).
var reverse = map[byte][2]string{
	'k': {"meng", "peng"},
	'p': {"mem", "pem"},
	't': {"men", "pen"},
	's': {"meny", "peny"},
}

// Variants mengembalikan bentuk alternatif kata (huruf kecil), tanpa kata aslinya, unik dan berurutan
// deterministik. nil bila kata < 4 rune atau tidak ada aturan yang berlaku.
func Variants(word string) []string {
	if len([]rune(word)) < minRunes || word == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{word: true}
	add := func(v string) {
		if len([]rune(v)) < 3 || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}

	forward := false
	for _, p := range prefixes {
		if strings.HasPrefix(word, p.prefix) && isVowelAt(word, len(p.prefix)) {
			add(p.lead + word[len(p.prefix):])
			forward = true
			break
		}
	}
	if !forward && len(word) > 1 && isVowelAt(word, 1) {
		if pair, ok := reverse[word[0]]; ok {
			rest := word[1:]
			add(pair[0] + rest)
			add(pair[1] + rest)
		}
	}
	return out
}
