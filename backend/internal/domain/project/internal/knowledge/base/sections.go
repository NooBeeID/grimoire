package base

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"grimoire/lib/util/mdoutline"
)

// MaxSections membatasi jumlah bagian per file yang disimpan di kolom sections.
const MaxSections = 300

// Section adalah satu bagian dokumen: dari satu heading sampai heading berikutnya (level apa pun).
// Start/End adalah offset RUNE (bukan byte) pada isi, [Start, End), supaya substr SQL memotong tepat.
type Section struct {
	Heading string `json:"h"`
	Start   int    `json:"s"`
	End     int    `json:"e"`
}

// Sections membagi isi menurut heading Markdown (parser mdoutline T18). Isi sebelum heading pertama
// yang tidak kosong menjadi bagian dengan Heading "". Lebih dari MaxSections: bagian terakhir yang
// disimpan diperpanjang sampai akhir isi. Tanpa heading: satu bagian "" seluruh isi. Isi kosong: [].
func Sections(content string) []Section {
	if content == "" {
		return []Section{}
	}
	total := utf8.RuneCountInString(content)
	headings := mdoutline.Parse(content)
	if len(headings) == 0 {
		return []Section{{Heading: "", Start: 0, End: total}}
	}

	// offset byte -> rune, bertahap karena heading berurutan.
	runeAt := func() func(byteOff int) int {
		lastByte, lastRune := 0, 0
		return func(b int) int {
			if b < lastByte {
				lastByte, lastRune = 0, 0
			}
			lastRune += utf8.RuneCountInString(content[lastByte:b])
			lastByte = b
			return lastRune
		}
	}()

	var out []Section
	if first := headings[0].Start; strings.TrimSpace(content[:first]) != "" {
		out = append(out, Section{Heading: "", Start: 0, End: runeAt(first)})
	}
	for i, h := range headings {
		s := runeAt(h.Start)
		e := total
		if i+1 < len(headings) {
			e = runeAt(headings[i+1].Start)
		}
		out = append(out, Section{Heading: h.Text, Start: s, End: e})
	}
	if len(out) > MaxSections {
		out = out[:MaxSections]
		out[MaxSections-1].End = total
	}
	return out
}

// SectionsJSON mengembalikan Sections sebagai array JSON (kolom sections jsonb); "[]" untuk isi kosong.
func SectionsJSON(content string) []byte {
	b, err := json.Marshal(Sections(content))
	if err != nil { // tidak mungkin: hanya string dan int
		return []byte("[]")
	}
	return b
}
