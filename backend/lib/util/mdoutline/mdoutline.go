// Package mdoutline mengurai heading Markdown menjadi daftar (level, teks, baris, rentang byte).
// Dipakai tool MCP get_file_outline dan read_section (T18), dan dipakai lagi oleh T20.
//
// Aturan: ATX (# sampai ######) dan Setext; fenced code (``` dan ~~~), blok HTML, indented code, dan YAML
// front matter diabaikan; baris dihitung dari 1; offset byte dihitung dari isi mentah (CRLF dan BOM utuh),
// sedangkan Text tidak memuat \r maupun BOM. Hanya stdlib (lib netral, rules 4.9).
package mdoutline

import (
	"strings"
)

// Heading satu heading Markdown.
type Heading struct {
	Level int    // 1..6 (Setext: '=' -> 1, '-' -> 2)
	Text  string // trim spasi; ATX tanpa penutup " ###"; Setext multi-baris digabung satu spasi
	Line  int    // baris pertama heading, mulai 1 (Setext: baris pertama paragraf)
	Start int    // offset byte awal baris heading di isi mentah
	End   int    // offset byte eksklusif: Start heading berikutnya dengan Level <= Level ini, atau len(content)
}

const bom = "\ufeff"

type line struct {
	start int
	text  string // tanpa \n dan \r akhir (BOM dibuang pada baris 1)
}

func splitLines(content string) []line {
	var out []line
	pos := 0
	for pos < len(content) {
		end := strings.IndexByte(content[pos:], '\n')
		var raw string
		next := 0
		if end < 0 {
			raw = content[pos:]
			next = len(content)
		} else {
			raw = content[pos : pos+end]
			next = pos + end + 1
		}
		raw = strings.TrimSuffix(raw, "\r")
		if pos == 0 {
			raw = strings.TrimPrefix(raw, bom)
		}
		out = append(out, line{start: pos, text: raw})
		pos = next
	}
	return out
}

// indent mengembalikan jumlah kolom indentasi (tab = 4) dan sisa teks.
func indent(s string) (int, string) {
	cols := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			cols++
		case '\t':
			cols += 4 - cols%4
		default:
			return cols, s[i:]
		}
	}
	return cols, ""
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

// atx mengurai baris ATX (setelah indentasi dibuang). ok=false bila bukan heading.
func atx(rest string) (level int, text string, ok bool) {
	n := 0
	for n < len(rest) && rest[n] == '#' {
		n++
	}
	if n < 1 || n > 6 {
		return 0, "", false
	}
	if n < len(rest) && rest[n] != ' ' && rest[n] != '\t' {
		return 0, "", false
	}
	body := strings.TrimSpace(rest[n:])
	// buang urutan penutup: '#' di akhir yang didahului spasi/tab, atau seluruh isi hanya '#'.
	t := strings.TrimRight(body, "#")
	if t == "" {
		body = ""
	} else if t != body && (t[len(t)-1] == ' ' || t[len(t)-1] == '\t') {
		body = strings.TrimSpace(t)
	}
	return n, body, true
}

// fenceOpen mengenali pembuka fence; mengembalikan karakter dan panjangnya.
func fenceOpen(rest string) (ch byte, n int, ok bool) {
	if rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return 0, 0, false
	}
	ch = rest[0]
	for n < len(rest) && rest[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	if ch == '`' && strings.IndexByte(rest[n:], '`') >= 0 {
		return 0, 0, false // info string backtick bukan fence
	}
	return ch, n, true
}

func fenceCloses(rest string, ch byte, minLen int) bool {
	n := 0
	for n < len(rest) && rest[n] == ch {
		n++
	}
	return n >= minLen && isBlank(rest[n:])
}

func setextLevel(rest string) int {
	t := strings.TrimRight(rest, " \t")
	if t == "" {
		return 0
	}
	c := t[0]
	if c != '=' && c != '-' {
		return 0
	}
	for i := 0; i < len(t); i++ {
		if t[i] != c {
			return 0
		}
	}
	if c == '=' {
		return 1
	}
	return 2
}

func isListItem(rest string) bool {
	if rest == "" {
		return false
	}
	switch rest[0] {
	case '-', '*', '+':
		return len(rest) == 1 || rest[1] == ' ' || rest[1] == '\t'
	}
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == 0 || i > 9 || i >= len(rest) {
		return false
	}
	if rest[i] != '.' && rest[i] != ')' {
		return false
	}
	return i+1 == len(rest) || rest[i+1] == ' ' || rest[i+1] == '\t'
}

func isHTMLStart(rest string) bool {
	if len(rest) < 2 || rest[0] != '<' {
		return false
	}
	c := rest[1]
	return c == '/' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func frontMatterEnd(lines []line) int {
	if len(lines) == 0 || strings.TrimRight(lines[0].text, " \t") != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		t := strings.TrimRight(lines[i].text, " \t")
		if t == "---" || t == "..." {
			return i + 1
		}
	}
	return 0
}

// Parse mengembalikan semua heading berurutan sesuai kemunculan; tidak pernah nil.
func Parse(content string) []Heading {
	hs := []Heading{}
	lines := splitLines(content)
	i := frontMatterEnd(lines)

	var (
		inFence    bool
		fenceCh    byte
		fenceLen   int
		inComment  bool
		inHTML     bool
		noPara     bool // di dalam list/blockquote: baris lanjutan bukan paragraf baru
		paraActive bool
		paraStart  int
		paraLine   int
		paraParts  []string
	)
	endPara := func() { paraActive = false; paraParts = nil }

	for ; i < len(lines); i++ {
		ln := lines[i]
		cols, rest := indent(ln.text)

		if inFence {
			if cols < 4 && fenceCloses(rest, fenceCh, fenceLen) {
				inFence = false
			}
			continue
		}
		if inComment {
			if strings.Contains(ln.text, "-->") {
				inComment = false
			}
			continue
		}
		if inHTML {
			if isBlank(ln.text) {
				inHTML = false
			}
			continue
		}
		if isBlank(ln.text) {
			endPara()
			noPara = false
			continue
		}
		if cols >= 4 {
			if paraActive {
				paraParts = append(paraParts, strings.TrimSpace(rest))
			}
			continue
		}
		if paraActive {
			if lvl := setextLevel(rest); lvl > 0 {
				hs = append(hs, Heading{Level: lvl, Text: strings.Join(paraParts, " "), Line: paraLine, Start: paraStart})
				endPara()
				continue
			}
		}
		if ch, n, ok := fenceOpen(rest); ok {
			endPara()
			inFence, fenceCh, fenceLen = true, ch, n
			continue
		}
		if lvl, text, ok := atx(rest); ok {
			endPara()
			noPara = false
			hs = append(hs, Heading{Level: lvl, Text: text, Line: i + 1, Start: ln.start})
			continue
		}
		if strings.HasPrefix(rest, "<!--") {
			endPara()
			if !strings.Contains(rest[4:], "-->") {
				inComment = true
			}
			continue
		}
		if !paraActive && isHTMLStart(rest) {
			inHTML = true
			continue
		}
		if rest[0] == '>' || isListItem(rest) {
			endPara()
			noPara = true
			continue
		}
		if noPara {
			continue
		}
		if !paraActive {
			paraActive, paraStart, paraLine = true, ln.start, i+1
		}
		paraParts = append(paraParts, strings.TrimSpace(rest))
	}

	for a := range hs {
		hs[a].End = len(content)
		for b := a + 1; b < len(hs); b++ {
			if hs[b].Level <= hs[a].Level {
				hs[a].End = hs[b].Start
				break
			}
		}
	}
	return hs
}

// Find mencari heading dengan teks persis (setelah TrimSpace) pada kemunculan ke-occurrence (mulai 1).
// count = jumlah heading bernama sama; ok = 1 <= occurrence <= count.
func Find(hs []Heading, text string, occurrence int) (h Heading, count int, ok bool) {
	text = strings.TrimSpace(text)
	for _, c := range hs {
		if c.Text != text {
			continue
		}
		count++
		if count == occurrence {
			h, ok = c, true
		}
	}
	if !ok {
		return Heading{}, count, false
	}
	return h, count, true
}
