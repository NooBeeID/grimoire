// Package base memuat entity dan rule bisnis gateway MCP tanpa I/O.
package base

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"

	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// ReservedToolNames adalah nama yang bentrok dengan aksi audit gateway sendiri.
var ReservedToolNames = []string{"initialize", "tools_list", "unknown_tool", "auth", "rate_limited"}

var toolNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Catalog adalah kumpulan tool tervalidasi, terurut nama.
type Catalog struct {
	tools []toolkit.Tool
	index map[string]int
}

// NewCatalog memvalidasi tools dan membangun katalog. Daftar kosong valid.
func NewCatalog(tools []toolkit.Tool) (Catalog, error) {
	actions := record.Actions()
	seen := make(map[string]bool, len(tools))
	sorted := make([]toolkit.Tool, 0, len(tools))
	for _, t := range tools {
		switch {
		case !toolNameRe.MatchString(t.Name):
			return Catalog{}, fmt.Errorf("nama tool MCP tidak valid: %q", t.Name)
		case slices.Contains(ReservedToolNames, t.Name):
			return Catalog{}, fmt.Errorf("nama tool MCP terlarang: %q", t.Name)
		case !slices.Contains(actions, "mcp."+t.Name):
			return Catalog{}, fmt.Errorf("aksi audit %q untuk tool %q belum ada di katalog record.Actions()", "mcp."+t.Name, t.Name)
		case seen[t.Name]:
			return Catalog{}, fmt.Errorf("tool MCP duplikat: %q", t.Name)
		case t.Handler == nil:
			return Catalog{}, fmt.Errorf("tool MCP %q: handler nil", t.Name)
		case !validSchema(t.InputSchema):
			return Catalog{}, fmt.Errorf("tool MCP %q: inputSchema harus JSON object dengan \"type\":\"object\"", t.Name)
		}
		seen[t.Name] = true
		sorted = append(sorted, t)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	idx := make(map[string]int, len(sorted))
	for i, t := range sorted {
		idx[t.Name] = i
	}
	return Catalog{tools: sorted, index: idx}, nil
}

func validSchema(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return false
	}
	var typ string
	return json.Unmarshal(m["type"], &typ) == nil && typ == "object"
}

// List mengembalikan salinan tool terurut nama.
func (c Catalog) List() []toolkit.Tool { return slices.Clone(c.tools) }

// Lookup mencari tool berdasarkan nama.
func (c Catalog) Lookup(name string) (toolkit.Tool, bool) {
	i, ok := c.index[name]
	if !ok {
		return toolkit.Tool{}, false
	}
	return c.tools[i], true
}

// Len jumlah tool.
func (c Catalog) Len() int { return len(c.tools) }

// Names nama tool terurut.
func (c Catalog) Names() []string {
	out := make([]string, len(c.tools))
	for i, t := range c.tools {
		out[i] = t.Name
	}
	return out
}
