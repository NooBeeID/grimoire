package openapi

import "go.yaml.in/yaml/v3"

type metric struct{ size, height int }

type guardState struct {
	lim      Limits
	aliases  int
	memo     map[*yaml.Node]metric
	visiting map[*yaml.Node]bool
	bad      bool
}

func (g *guardState) fail(_ *yaml.Node) {
	if !g.bad {
		g.bad = true
	}
}

// walk menghitung ukuran (setelah ekspansi alias, jenuh di MaxNodes+1) dan tinggi pohon ter-ekspansi.
// Memo per node membuat biayanya O(node asli). rel adalah kedalaman relatif dari awal penelusuran ini.
func (g *guardState) walk(n *yaml.Node, rel int) metric {
	if g.bad {
		return metric{}
	}
	if rel > g.lim.MaxDepth {
		g.fail(n)
		return metric{}
	}
	if m, ok := g.memo[n]; ok {
		return m
	}
	if g.visiting[n] {
		g.fail(n)
		return metric{}
	}
	g.visiting[n] = true
	defer delete(g.visiting, n)

	var m metric
	switch n.Kind {
	case yaml.AliasNode:
		g.aliases++
		if g.aliases > g.lim.MaxAliases || n.Alias == nil {
			g.fail(n)
			return metric{}
		}
		t := g.walk(n.Alias, rel+1)
		m = metric{size: t.size + 1, height: t.height + 1}
	default:
		m = metric{size: 1, height: 1}
		maxChild := 0
		for _, c := range n.Content {
			cm := g.walk(c, rel+1)
			if g.bad {
				return metric{}
			}
			m.size += cm.size
			if m.size > g.lim.MaxNodes {
				m.size = g.lim.MaxNodes + 1
			}
			if cm.height > maxChild {
				maxChild = cm.height
			}
		}
		m.height += maxChild
	}
	if m.size > g.lim.MaxNodes || m.height > g.lim.MaxDepth+1 {
		g.fail(n)
		return metric{}
	}
	g.memo[n] = m
	return m
}

// guard berjalan pada *yaml.Node SEBELUM konversi. Mengembalikan issue YAML_LIMIT bila kedalaman,
// jumlah alias, ukuran setelah ekspansi alias, atau siklus alias melewati batas.
func guard(n *yaml.Node, lim Limits) *Issue {
	g := &guardState{lim: lim, memo: map[*yaml.Node]metric{}, visiting: map[*yaml.Node]bool{}}
	g.walk(n, 0)
	if !g.bad {
		return nil
	}
	is := compose(CodeYAMLLimit, "dokumen YAML terlalu kompleks (alias/kedalaman)", "", 0)
	return &is
}
