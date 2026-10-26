package base

import (
	"fmt"
	"strings"
	"time"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/readfile"
	"grimoire/lib/util/mdoutline"
)

// maxHeadingDisplay membatasi teks heading di keluaran (sama dengan batas argumen heading read_section).
const maxHeadingDisplay = 500

const outlineFooter = "Baca satu bagian dengan read_section(path, heading); heading ganda pakai occurrence."

// IsMarkdown: ekstensi .md atau .markdown, tanpa membedakan huruf besar/kecil.
func IsMarkdown(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".markdown")
}

// Outline adalah kerangka heading satu file knowledge.
type Outline struct {
	Path      string
	SizeBytes int
	UpdatedAt time.Time
	Markdown  bool
	Headings  []mdoutline.Heading // semua
	Shown     int                 // jumlah yang muat budget teks
}

// NewOutline mengurai heading hanya bila path Markdown. budget adalah batas byte Text().
func NewOutline(path string, resp readfile.Response, budget int) Outline {
	o := Outline{Path: path, SizeBytes: resp.TotalBytes, UpdatedAt: resp.UpdatedAt, Markdown: IsMarkdown(path)}
	if !o.Markdown {
		return o
	}
	o.Headings = mdoutline.Parse(resp.Content)
	o.Shown = o.fit(budget)
	return o
}

func headingDisplay(h mdoutline.Heading) string {
	return TruncateUTF8(OneLine(h.Text), maxHeadingDisplay)
}

func headingLine(h mdoutline.Heading) string {
	return fmt.Sprintf("%sL%d %s %s [%d-%d]", strings.Repeat("  ", h.Level-1), h.Line,
		strings.Repeat("#", h.Level), headingDisplay(h), h.Start, h.End)
}

func (o Outline) header() string {
	return fmt.Sprintf("%s (%s, diperbarui %s): %d heading", o.Path, HumanSize(o.SizeBytes), FormatTime(o.UpdatedAt), len(o.Headings))
}

func (o Outline) marker(shown int) string {
	return fmt.Sprintf("[outline terpotong: menampilkan %d dari %d heading; sisanya mulai byte %d, baca dengan read_file offset=%d]",
		shown, len(o.Headings), o.Headings[shown].Start, o.Headings[shown].Start)
}

// fit menghitung jumlah heading terbanyak yang membuat Text() <= budget.
func (o Outline) fit(budget int) int {
	n := len(o.Headings)
	fixed := len(o.header()) + 1 + len(outlineFooter)
	total := fixed
	for _, h := range o.Headings {
		total += len(headingLine(h)) + 1
	}
	if total <= budget || n == 0 {
		return n
	}
	// terpotong: sisakan ruang penanda terpanjang (indeks terakhir memberi angka terbesar).
	reserve := len(o.marker(n-1)) + 1 + 4 // +4: selisih digit angka Shown
	used := fixed + reserve
	shown := 0
	for _, h := range o.Headings {
		l := len(headingLine(h)) + 1
		if used+l > budget {
			break
		}
		used += l
		shown++
	}
	return shown
}

func (o Outline) Truncated() bool { return o.Shown < len(o.Headings) }

// Text memformat outline untuk LLM.
func (o Outline) Text() string {
	if !o.Markdown {
		return fmt.Sprintf("%s bukan file Markdown; outline kosong (%s). Baca dengan read_file.", o.Path, HumanSize(o.SizeBytes))
	}
	if len(o.Headings) == 0 {
		return fmt.Sprintf("Tidak ada heading Markdown di %s (%s). Baca dengan read_file.", o.Path, HumanSize(o.SizeBytes))
	}
	lines := []string{o.header()}
	for _, h := range o.Headings[:o.Shown] {
		lines = append(lines, headingLine(h))
	}
	if o.Truncated() {
		lines = append(lines, o.marker(o.Shown))
	}
	lines = append(lines, outlineFooter)
	return strings.Join(lines, "\n")
}

// Structured: hanya heading yang ditampilkan.
func (o Outline) Structured() map[string]any {
	hs := make([]map[string]any, 0, o.Shown)
	for _, h := range o.Headings[:o.Shown] {
		hs = append(hs, map[string]any{
			"level": h.Level, "text": headingDisplay(h), "line": h.Line, "start": h.Start, "end": h.End,
		})
	}
	return map[string]any{
		"path": o.Path, "size_bytes": o.SizeBytes, "updated_at": RFC3339(o.UpdatedAt),
		"markdown": o.Markdown, "total_headings": len(o.Headings), "truncated": o.Truncated(),
		"headings": hs,
	}
}

// Summary adalah metadata audit (tanpa isi, tanpa nilai string).
func (o Outline) Summary() map[string]any {
	return map[string]any{"count": o.Shown, "total": len(o.Headings), "bytes": o.SizeBytes, "truncated": o.Truncated()}
}

func (o Outline) Result() toolkit.Result {
	return toolkit.Result{Text: o.Text(), TargetType: "file", Target: o.Path, Summary: o.Summary(), Structured: o.Structured()}
}
