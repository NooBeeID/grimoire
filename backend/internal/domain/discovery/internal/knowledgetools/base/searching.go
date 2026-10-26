package base

import (
	"encoding/json"
	"fmt"
	"strings"

	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/search"
)

// SearchHits adalah hasil pencarian knowledge yang siap dirender untuk LLM. Murni: tanpa I/O.
type SearchHits struct {
	Query  string
	Prefix string // "" = seluruh project
	Items  []search.Item
	Total  int // jumlah dokumen cocok sebelum limit; selalu >= len(Items)
	// Expanded true bila pencarian memakai sinonim atau koreksi typo (T20).
	Expanded bool
}

// NewSearchHits membangun hasil; Total = max(resp.Total, len(resp.Items)).
func NewSearchHits(query, prefix string, resp search.Response) SearchHits {
	return SearchHits{
		Query: query, Prefix: prefix,
		Items: append([]search.Item(nil), resp.Items...),
		Total: max(resp.Total, len(resp.Items)), Expanded: resp.Expanded,
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
	hasSection := false
	for i, it := range h.Items {
		lines = append(lines, fmt.Sprintf("%d. %s  (skor %.3f)", i+1, it.Path, it.Score))
		if sn := strings.TrimSpace(OneLine(it.Snippet)); sn != "" {
			lines = append(lines, "   "+sn)
		}
		if it.Section != "" {
			hasSection = true
			lines = append(lines, "   bagian: "+OneLine(it.Section))
		}
	}
	if h.Total > len(h.Items) {
		lines = append(lines, fmt.Sprintf("[ada %d hasil lain; persempit query atau path, atau naikkan limit (maks 20)]", h.Total-len(h.Items)))
	}
	if hasSection {
		lines = append(lines, "Baca isi lengkap dengan read_file(path), atau satu bagian dengan read_section(path, heading).")
	} else {
		lines = append(lines, "Baca isi lengkap dengan read_file(path).")
	}
	return strings.Join(lines, "\n")
}

// Structured merender structuredContent.
func (h SearchHits) Structured() map[string]any {
	items := make([]map[string]any, 0, len(h.Items))
	for _, it := range h.Items {
		m := map[string]any{"path": it.Path, "score": it.Score, "snippet": it.Snippet}
		if it.Section != "" {
			m["section"] = it.Section
		}
		items = append(items, m)
	}
	return map[string]any{"items": items, "total": h.Total, "query": h.Query}
}

const (
	MaxResultPaths         = 10   // K1
	ResultPathsBudgetBytes = 6144 // anggaran JSON result_paths; sisa ~2 KB untuk kunci audit lain
)

// ResultPaths mengembalikan path hasil teratas untuk audit, urut sama dengan Items.
// Maks MaxResultPaths; tiap path TruncateUTF8(path, record.MaxTargetBytes);
// berhenti bila total panjang JSON (kurung siku + sum len(json.Marshal(p)) + koma) melewati
// ResultPathsBudgetBytes. Tidak pernah nil. Bool kedua true bila ada path dibuang karena
// anggaran (bukan karena batas MaxResultPaths).
func (h SearchHits) ResultPaths() ([]string, bool) {
	limit := min(len(h.Items), MaxResultPaths)
	paths := make([]string, 0, limit)
	size := 2 // "[" dan "]"
	for i := 0; i < limit; i++ {
		p := TruncateUTF8(h.Items[i].Path, record.MaxTargetBytes)
		enc, err := json.Marshal(p)
		if err != nil {
			return paths, true
		}
		add := len(enc)
		if len(paths) > 0 {
			add++ // koma
		}
		if size+add > ResultPathsBudgetBytes {
			return paths, true
		}
		size += add
		paths = append(paths, p)
	}
	return paths, false
}

// Summary adalah metadata audit: count, total, dan result_paths (path hasil teratas, maks 10).
// Path adalah metadata (sudah tercatat sebagai target audit read_file), bukan isi file. T11 membalik
// keputusan T08 ("tanpa path hasil") karena metrik follow-up dan rank dibuka butuh urutan hasil.
// Snippet, skor, isi file, dan query TIDAK masuk (query sudah ada di args.query).
// result_paths_truncated hanya ada (true) bila path dibuang karena anggaran ukuran audit.
func (h SearchHits) Summary() map[string]any {
	paths, truncated := h.ResultPaths()
	m := map[string]any{"count": len(h.Items), "total": h.Total, "result_paths": paths}
	if truncated {
		m["result_paths_truncated"] = true
	}
	if h.Expanded {
		m["expanded"] = true
	}
	return m
}

// Result merangkai toolkit.Result.
func (h SearchHits) Result() toolkit.Result {
	return toolkit.Result{
		Text: h.Text(), Structured: h.Structured(),
		TargetType: "folder", Target: h.Prefix, Summary: h.Summary(),
	}
}
