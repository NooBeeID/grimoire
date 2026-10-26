package base

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/listspecs"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

// MaxChangesWindow adalah jendela terjauh since terhadap sekarang.
const MaxChangesWindow = 90 * 24 * time.Hour

const (
	ChangeKindFile = "file"
	ChangeKindSpec = "spec"
)

// Change adalah satu file atau spec yang berubah. Identitas pengubah sengaja tidak ada (K5).
type Change struct {
	Kind      string // "file" | "spec"
	Path      string // kind file
	Name      string // kind spec
	UpdatedAt time.Time
	Created   bool // CreatedAt.Equal(UpdatedAt)
	SizeBytes int
}

func (c Change) key() string {
	if c.Kind == ChangeKindFile {
		return c.Path
	}
	return c.Name
}

// ChangePage adalah halaman perubahan gabungan file dan spec.
type ChangePage struct {
	Since time.Time
	Kind  string // "files" | "specs" | "all"
	Items []Change
	Total int
}

// MergeChanges menggabungkan file (sudah difilter dan dibatasi provider) dan spec (difilter di sini menurut since).
// Urut UpdatedAt menurun, lalu Kind (file sebelum spec), lalu Path/Name menurut urutan byte.
func MergeChanges(since time.Time, kind string, files listchanges.Response, specs []listspecs.Spec, limit int) ChangePage {
	items := make([]Change, 0, len(files.Items)+len(specs))
	for _, f := range files.Items {
		items = append(items, Change{Kind: ChangeKindFile, Path: f.Path, UpdatedAt: f.UpdatedAt.UTC(),
			Created: f.CreatedAt.Equal(f.UpdatedAt), SizeBytes: f.SizeBytes})
	}
	matched := 0
	for _, s := range specs {
		if s.UpdatedAt.Before(since) {
			continue
		}
		matched++
		items = append(items, Change{Kind: ChangeKindSpec, Name: s.Name, UpdatedAt: s.UpdatedAt.UTC(),
			Created: s.CreatedAt.Equal(s.UpdatedAt), SizeBytes: s.SizeBytes})
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.key() < b.key()
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return ChangePage{Since: since.UTC(), Kind: kind, Items: items, Total: files.Total + matched}
}

func (p ChangePage) Truncated() bool { return p.Total > len(p.Items) }

func (p ChangePage) Text() string {
	since := RFC3339(p.Since)
	if len(p.Items) == 0 {
		return "Tidak ada perubahan sejak " + since + "."
	}
	lines := []string{fmt.Sprintf("%d perubahan sejak %s (total %d):", len(p.Items), since, p.Total)}
	for _, c := range p.Items {
		verb := "diperbarui"
		if c.Created {
			verb = "dibuat"
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s · %s · %s · %s", c.Kind, c.key(), verb, HumanSize(c.SizeBytes), FormatTime(c.UpdatedAt)))
	}
	if p.Truncated() {
		lines = append(lines, fmt.Sprintf("[ada %d perubahan lagi; persempit since/path/kind atau naikkan limit (maks 50)]", p.Total-len(p.Items)))
	}
	return strings.Join(lines, "\n")
}

func (p ChangePage) Structured() map[string]any {
	items := make([]map[string]any, 0, len(p.Items))
	for _, c := range p.Items {
		m := map[string]any{"kind": c.Kind, "updated_at": RFC3339(c.UpdatedAt), "created": c.Created, "size_bytes": c.SizeBytes}
		if c.Kind == ChangeKindFile {
			m["path"] = c.Path
		} else {
			m["name"] = c.Name
		}
		items = append(items, m)
	}
	return map[string]any{
		"since": RFC3339(p.Since), "kind": p.Kind, "total": p.Total, "truncated": p.Truncated(), "items": items,
	}
}

func (p ChangePage) Summary() map[string]any {
	return map[string]any{"count": len(p.Items), "total": p.Total}
}

// Result: tanpa target audit (list_recent_changes tidak menunjuk satu objek).
func (p ChangePage) Result() toolkit.Result {
	return toolkit.Result{Text: p.Text(), Summary: p.Summary(), Structured: p.Structured()}
}
