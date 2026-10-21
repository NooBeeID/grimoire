package base

import (
	"fmt"
	"strings"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/readfile"
)

const (
	reserveBytes     = 512 // cadangan untuk penanda dan selisih rune di bawah MCPMaxResponseBytes
	minReadLimit     = 1024
	defaultReadLimit = 40000
	maxSimilar       = 5
)

// ReadOutcome adalah hasil usecase read_file: ReadChunk atau NotFound.
type ReadOutcome interface {
	Result() toolkit.Result
}

// MaxReadLimit = max(1024, maxResponseBytes-512).
func MaxReadLimit(maxResponseBytes int) int {
	return max(minReadLimit, maxResponseBytes-reserveBytes)
}

// DefaultReadLimit = min(40000, max).
func DefaultReadLimit(max int) int { return min(defaultReadLimit, max) }

// ReadChunk adalah satu potongan isi file.
type ReadChunk struct {
	Path       string
	Offset     int // offset yang diminta caller
	Content    string
	SizeBytes  int
	TotalBytes int
	NextOffset int
	Truncated  bool
}

// NewReadChunk menyalin field dari resp; Offset dari argumen. Tidak mengubah Content.
func NewReadChunk(resp readfile.Response, offset int) ReadChunk {
	return ReadChunk{
		Path: resp.Path, Offset: offset, Content: resp.Content, SizeBytes: resp.SizeBytes,
		TotalBytes: resp.TotalBytes, NextOffset: resp.NextOffset, Truncated: resp.Truncated,
	}
}

// Text: isi apa adanya, ditambah penanda lanjutan bila terpotong (D10).
func (c ReadChunk) Text() string {
	if c.TotalBytes == 0 {
		return "[file kosong: " + c.Path + "]"
	}
	if c.Content == "" {
		return fmt.Sprintf("[tidak ada isi pada offset=%d; file berukuran %d byte]", c.Offset, c.TotalBytes)
	}
	if c.Truncated {
		return c.Content + fmt.Sprintf("\n\n[terpotong: lanjutkan dengan offset=%d dari total %d byte]", c.NextOffset, c.TotalBytes)
	}
	return c.Content
}

// Summary adalah metadata audit (tanpa isi).
func (c ReadChunk) Summary() map[string]any {
	next := -1
	if c.Truncated {
		next = c.NextOffset
	}
	return map[string]any{
		"found": true, "bytes": c.SizeBytes, "total_bytes": c.TotalBytes,
		"offset": c.Offset, "next_offset": next, "truncated": c.Truncated,
	}
}

// Result merangkai toolkit.Result (tanpa Structured).
func (c ReadChunk) Result() toolkit.Result {
	return toolkit.Result{Text: c.Text(), TargetType: "file", Target: c.Path, Summary: c.Summary()}
}

// NotFound adalah hasil file tidak ditemukan beserta saran path.
type NotFound struct {
	Path    string
	Similar []string // maks 5
}

// NewNotFound memotong saran ke 5 dan menyalin slice.
func NewNotFound(path string, similar []string) NotFound {
	if len(similar) > maxSimilar {
		similar = similar[:maxSimilar]
	}
	return NotFound{Path: path, Similar: append([]string(nil), similar...)}
}

func (n NotFound) Text() string {
	lines := []string{"File tidak ditemukan: " + n.Path}
	if len(n.Similar) > 0 {
		lines = append(lines, "Similar:")
		for _, s := range n.Similar {
			lines = append(lines, "- "+s)
		}
	}
	lines = append(lines, "Gunakan list_files untuk melihat struktur folder atau search_knowledge untuk mencari berdasarkan isi.")
	return strings.Join(lines, "\n")
}

func (n NotFound) Summary() map[string]any {
	return map[string]any{"found": false, "similar_count": len(n.Similar), "error_code": "not_found"}
}

func (n NotFound) Result() toolkit.Result {
	return toolkit.Result{Text: n.Text(), IsError: true, TargetType: "file", Target: n.Path, Summary: n.Summary()}
}
