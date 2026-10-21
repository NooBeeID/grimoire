package base

import (
	"fmt"
	"strings"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

// ListPage adalah satu halaman hasil list_files. Murni, tanpa I/O.
type ListPage struct {
	Prefix    string // sudah dinormalisasi, "" = akar
	Recursive bool
	Offset    int
	Items     []listfiles.Item
	Folders   []string // hanya terisi bila !Recursive && Offset == 0
	Total     int      // jumlah file yang cocok (bukan hanya halaman ini)
}

// NewListPage membangun halaman dari respons T05.
func NewListPage(prefix string, recursive bool, offset int, resp listfiles.Response) ListPage {
	p := ListPage{
		Prefix:    prefix,
		Recursive: recursive,
		Offset:    offset,
		Items:     append([]listfiles.Item(nil), resp.Items...),
		Total:     resp.Total,
	}
	if !recursive && offset == 0 {
		for _, f := range resp.Folders {
			if f = strings.Trim(f, "/"); f != "" {
				p.Folders = append(p.Folders, f)
			}
		}
	}
	return p
}

// NextOffset mengembalikan offset halaman berikutnya, -1 bila habis.
func (p ListPage) NextOffset() int {
	if n := p.Offset + len(p.Items); n < p.Total {
		return n
	}
	return -1
}

// OffsetOutOfRange benar bila offset melewati seluruh file.
func (p ListPage) OffsetOutOfRange() bool { return p.Total > 0 && p.Offset >= p.Total }

func (p ListPage) isEmpty() bool { return p.Total == 0 && len(p.Folders) == 0 }

// Text merender teks untuk LLM.
func (p ListPage) Text() string {
	if p.OffsetOutOfRange() {
		return fmt.Sprintf("Offset %d melewati jumlah file (%d). Gunakan offset 0-%d.", p.Offset, p.Total, p.Total-1)
	}
	if p.isEmpty() {
		if p.Prefix == "" {
			return "Belum ada file knowledge di project ini."
		}
		return fmt.Sprintf("Tidak ada file di folder '%s'. Panggil list_files tanpa path untuk melihat struktur dari akar, atau gunakan search_knowledge.", p.Prefix)
	}
	folder := "/ (akar)"
	if p.Prefix != "" {
		folder = p.Prefix
	}
	rec := "tidak"
	if p.Recursive {
		rec = "ya"
	}
	lines := []string{fmt.Sprintf("Folder: %s | rekursif: %s | %d file", folder, rec, p.Total)}
	for _, f := range p.Folders {
		lines = append(lines, "[folder] "+f+"/")
	}
	for _, it := range p.Items {
		lines = append(lines, fmt.Sprintf("%s  %s  %s", it.Path, HumanSize(it.SizeBytes), FormatTime(it.UpdatedAt)))
	}
	out := strings.Join(lines, "\n")
	if next := p.NextOffset(); next >= 0 {
		suffix := ""
		if p.Prefix != "" {
			suffix += " path=" + p.Prefix
		}
		if p.Recursive {
			suffix += " recursive=true"
		}
		out += fmt.Sprintf("\n\n[ada %d file lagi, panggil list_files dengan offset=%d%s]", p.Total-next, next, suffix)
	}
	return out
}

// Structured merender structuredContent.
func (p ListPage) Structured() map[string]any {
	items := make([]map[string]any, 0, len(p.Items))
	for _, it := range p.Items {
		items = append(items, map[string]any{
			"path":       it.Path,
			"size_bytes": it.SizeBytes,
			"updated_at": RFC3339(it.UpdatedAt),
		})
	}
	folders := make([]string, 0, len(p.Folders))
	folders = append(folders, p.Folders...)
	var next any
	if n := p.NextOffset(); n >= 0 {
		next = n
	}
	return map[string]any{
		"items":       items,
		"folders":     folders,
		"total":       p.Total,
		"next_offset": next,
		"recursive":   p.Recursive,
	}
}

// Summary adalah metadata audit (tanpa path/isi).
func (p ListPage) Summary() map[string]any {
	return map[string]any{
		"count":       len(p.Items),
		"total":       p.Total,
		"folders":     len(p.Folders),
		"next_offset": p.NextOffset(),
		"recursive":   p.Recursive,
	}
}

// Result merangkai toolkit.Result.
func (p ListPage) Result() toolkit.Result {
	if p.OffsetOutOfRange() {
		return toolkit.Result{
			Text:       p.Text(),
			IsError:    true,
			TargetType: "folder",
			Target:     p.Prefix,
			Summary: map[string]any{
				"error_code": "offset_out_of_range", "count": 0, "total": p.Total,
				"folders": 0, "next_offset": -1, "recursive": p.Recursive,
			},
		}
	}
	return toolkit.Result{
		Text:       p.Text(),
		Structured: p.Structured(),
		TargetType: "folder",
		Target:     p.Prefix,
		Summary:    p.Summary(),
	}
}
