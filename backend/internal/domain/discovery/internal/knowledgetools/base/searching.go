package base

import (
	"fmt"
	"strings"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/search"
)

// SearchHits adalah hasil pencarian knowledge yang siap dirender untuk LLM. Murni: tanpa I/O.
type SearchHits struct {
	Query  string
	Prefix string // "" = seluruh project
	Items  []search.Item
	Total  int // jumlah dokumen cocok sebelum limit; selalu >= len(Items)
}

// NewSearchHits membangun hasil; Total = max(resp.Total, len(resp.Items)).
func NewSearchHits(query, prefix string, resp search.Response) SearchHits {
	return SearchHits{
		Query: query, Prefix: prefix,
		Items: append([]search.Item(nil), resp.Items...),
		Total: max(resp.Total, len(resp.Items)),
	}
}

func (h SearchHits) scope() string {
	if h.Prefix == "" {
		return ""
	}
	return " di " + h.Prefix + "/"
}

// Text merender teks untuk LLM.
func (h SearchHits) Text() string {
	if len(h.Items) == 0 {
		return strings.Join([]string{
			fmt.Sprintf("Tidak ada hasil untuk \"%s\"%s", h.Query, h.scope()),
			"Coba kata kunci lain yang lebih umum atau sinonim, kurangi jumlah kata, hapus filter path, atau jelajahi struktur dengan list_files.",
		}, "\n")
	}
	lines := []string{fmt.Sprintf("%d dari %d hasil untuk \"%s\"%s", len(h.Items), h.Total, h.Query, h.scope())}
	for i, it := range h.Items {
		lines = append(lines, fmt.Sprintf("%d. %s  (skor %.3f)", i+1, it.Path, it.Score))
		if sn := strings.TrimSpace(OneLine(it.Snippet)); sn != "" {
			lines = append(lines, "   "+sn)
		}
	}
	if h.Total > len(h.Items) {
		lines = append(lines, fmt.Sprintf("[ada %d hasil lain; persempit query atau path, atau naikkan limit (maks 20)]", h.Total-len(h.Items)))
	}
	lines = append(lines, "Baca isi lengkap dengan read_file(path).")
	return strings.Join(lines, "\n")
}

// Structured merender structuredContent.
func (h SearchHits) Structured() map[string]any {
	items := make([]map[string]any, 0, len(h.Items))
	for _, it := range h.Items {
		items = append(items, map[string]any{"path": it.Path, "score": it.Score, "snippet": it.Snippet})
	}
	return map[string]any{"items": items, "total": h.Total, "query": h.Query}
}

// Summary adalah metadata audit (tanpa snippet, path hasil, maupun query).
func (h SearchHits) Summary() map[string]any {
	return map[string]any{"count": len(h.Items), "total": h.Total}
}

// Result merangkai toolkit.Result.
func (h SearchHits) Result() toolkit.Result {
	return toolkit.Result{
		Text: h.Text(), Structured: h.Structured(),
		TargetType: "folder", Target: h.Prefix, Summary: h.Summary(),
	}
}
