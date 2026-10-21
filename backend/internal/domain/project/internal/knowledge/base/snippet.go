package base

import "strings"

const (
	HighlightStart = "\x01"
	HighlightStop  = "\x02"
)

type Segment struct {
	Text      string `json:"text"`
	Highlight bool   `json:"highlight"`
}

// ParseHeadline memecah hasil ts_headline bermarker \x01..\x02 menjadi segmen. Marker tanpa pasangan
// diabaikan, segmen kosong dibuang, segmen bersebelahan berstatus sama digabung.
func ParseHeadline(s string) []Segment {
	segs := []Segment{}
	add := func(text string, hl bool) {
		if text == "" {
			return
		}
		if n := len(segs); n > 0 && segs[n-1].Highlight == hl {
			segs[n-1].Text += text
			return
		}
		segs = append(segs, Segment{Text: text, Highlight: hl})
	}
	var buf strings.Builder
	hl := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 0x01:
			if hl {
				continue // start ganda: abaikan
			}
			add(buf.String(), false)
			buf.Reset()
			hl = true
		case 0x02:
			if !hl {
				continue // stop yatim: abaikan
			}
			add(buf.String(), true)
			buf.Reset()
			hl = false
		default:
			buf.WriteByte(s[i])
		}
	}
	add(buf.String(), false) // sisa; start tanpa stop dianggap teks biasa
	return segs
}

// ToMarkdownBold mengganti \x01term\x02 menjadi **term** (untuk bus/agent); marker yatim dibuang.
func ToMarkdownBold(s string) string {
	var b strings.Builder
	for _, seg := range ParseHeadline(s) {
		if seg.Highlight {
			b.WriteString("**" + seg.Text + "**")
		} else {
			b.WriteString(seg.Text)
		}
	}
	return b.String()
}

type SearchHit struct {
	Path, Title string
	Score       float64
	Headline    string // mentah dengan marker
}

type SearchItemDTO struct {
	Path    string    `json:"path"`
	Title   string    `json:"title"`
	Score   float64   `json:"score"`
	Snippet []Segment `json:"snippet"`
}

type SearchResultDTO struct {
	Items []SearchItemDTO `json:"items"` // tidak pernah nil
	Total int             `json:"total"`
}

const (
	SearchDefaultLimit = 20
	SearchMaxLimit     = 50
)

// ClampLimit: n <= 0 -> def; n > max -> max.
func ClampLimit(n, def, max int) int {
	switch {
	case n <= 0:
		return def
	case n > max:
		return max
	}
	return n
}

// NewSearchResult membangun DTO HTTP dari hasil store (snippet dipecah jadi segmen).
func NewSearchResult(hits []SearchHit, total int) SearchResultDTO {
	items := make([]SearchItemDTO, 0, len(hits))
	for _, h := range hits {
		items = append(items, SearchItemDTO{Path: h.Path, Title: h.Title, Score: h.Score, Snippet: ParseHeadline(h.Headline)})
	}
	return SearchResultDTO{Items: items, Total: total}
}
