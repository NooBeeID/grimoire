package base

import (
	"fmt"
	"sort"
	"strings"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/util/mdoutline"
)

// Section adalah satu bagian file Markdown: dari heading sampai sebelum heading selevel atau lebih tinggi.
type Section struct {
	Path       string
	Heading    mdoutline.Heading
	Occurrence int
	Content    string // isi[Start : min(End, cut)], dipotong pada batas rune
	Truncated  bool
	NextOffset int // offset byte absolut untuk read_file bila Truncated, selain itu -1
}

// NewSection mengiris tepat [h.Start, h.End) dari content, maksimal limit byte (batas rune UTF-8).
func NewSection(path, content string, h mdoutline.Heading, occurrence, limit int) Section {
	raw := content[h.Start:h.End]
	cut := TruncateUTF8(raw, limit)
	s := Section{Path: path, Heading: h, Occurrence: occurrence, Content: cut, NextOffset: -1}
	if len(cut) < len(raw) {
		s.Truncated = true
		s.NextOffset = h.Start + len(cut)
	}
	return s
}

func (s Section) Text() string {
	if !s.Truncated {
		return s.Content
	}
	return s.Content + fmt.Sprintf("\n\n[bagian terpotong: lanjutkan dengan read_file offset=%d; bagian berakhir di byte %d]", s.NextOffset, s.Heading.End)
}

func (s Section) Result() toolkit.Result {
	return toolkit.Result{
		Text: s.Text(), TargetType: "file", Target: s.Path,
		Summary: map[string]any{"count": 1, "bytes": len(s.Content), "truncated": s.Truncated, "occurrence": s.Occurrence},
		Structured: map[string]any{
			"path": s.Path, "heading": s.Heading.Text, "level": s.Heading.Level, "line": s.Heading.Line,
			"occurrence": s.Occurrence, "start": s.Heading.Start, "end": s.Heading.End,
			"truncated": s.Truncated, "next_offset": s.NextOffset,
		},
	}
}

// HeadingMiss adalah hasil heading tidak ditemukan atau occurrence melebihi jumlah heading bernama sama.
type HeadingMiss struct {
	Path, Heading     string
	Occurrence, Count int      // Count = jumlah heading persis sama (0 bila tidak ada)
	Similar           []string // maks 5, format "<#*level> <text> (L<line>)"
}

// SimilarHeadings memberi saran heading mirip. Skor: sama (case-insensitive) 0 < saling memuat 1 <
// berbagi minimal satu kata 2; tanpa skor dibuang; seri diurut Line naik.
func SimilarHeadings(hs []mdoutline.Heading, text string, max int) []string {
	q := strings.ToLower(strings.TrimSpace(text))
	qWords := strings.Fields(q)
	type scored struct {
		score int
		h     mdoutline.Heading
	}
	var found []scored
	for _, h := range hs {
		t := strings.ToLower(h.Text)
		if t == "" || q == "" {
			continue
		}
		score := -1
		switch {
		case t == q:
			score = 0
		case strings.Contains(t, q) || strings.Contains(q, t):
			score = 1
		default:
			tw := map[string]bool{}
			for _, w := range strings.Fields(t) {
				tw[w] = true
			}
			for _, w := range qWords {
				if tw[w] {
					score = 2
					break
				}
			}
		}
		if score >= 0 {
			found = append(found, scored{score, h})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score < found[j].score
		}
		return found[i].h.Line < found[j].h.Line
	})
	out := []string{}
	for _, f := range found {
		if len(out) >= max {
			break
		}
		out = append(out, fmt.Sprintf("%s %s (L%d)", strings.Repeat("#", f.h.Level), headingDisplay(f.h), f.h.Line))
	}
	return out
}

func (m HeadingMiss) Result() toolkit.Result {
	var text string
	if m.Count > 0 {
		text = fmt.Sprintf("Heading %q hanya muncul %d kali di %s; gunakan occurrence 1 sampai %d.", OneLine(m.Heading), m.Count, m.Path, m.Count)
	} else {
		lines := []string{fmt.Sprintf("Heading tidak ditemukan di %s: \"%s\"", m.Path, OneLine(m.Heading))}
		if len(m.Similar) > 0 {
			lines = append(lines, "Mirip:")
			for _, s := range m.Similar {
				lines = append(lines, "- "+s)
			}
		}
		lines = append(lines, "Panggil get_file_outline untuk melihat semua heading.")
		text = strings.Join(lines, "\n")
	}
	return toolkit.Result{
		Text: text, IsError: true, TargetType: "file", Target: m.Path,
		Summary: map[string]any{"count": 0, "error_code": "heading_not_found", "similar_count": len(m.Similar)},
	}
}

// NotMarkdown adalah hasil read_section pada file yang bukan Markdown.
type NotMarkdown struct{ Path string }

func (n NotMarkdown) Result() toolkit.Result {
	return toolkit.Result{
		Text:    fmt.Sprintf("read_section hanya untuk file Markdown (.md, .markdown): %s. Baca dengan read_file.", n.Path),
		IsError: true, TargetType: "file", Target: n.Path,
		Summary: map[string]any{"count": 0, "error_code": "not_markdown"},
	}
}
