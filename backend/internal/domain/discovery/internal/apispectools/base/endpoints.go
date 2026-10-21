package base

import (
	"fmt"
	"strings"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/listendpoints"
)

const (
	closingReserve  = 200
	summaryMaxRunes = 160
	pathMaxRunes    = 300
	tagsMaxRunes    = 120
	opIDMaxRunes    = 100

	endpointDetailHint = "Detail: get_api_endpoint(spec, method, path) atau get_api_endpoint(spec, operation_id)."
)

// Filter adalah filter yang dipakai agent (sudah divalidasi/di-trim oleh Parse).
type Filter struct{ Spec, Tag, Query string }

// EndpointPage adalah satu halaman hasil list_api_endpoints. Murni, tanpa I/O.
type EndpointPage struct {
	Filter Filter
	Offset int
	Total  int
	Items  []listendpoints.Endpoint // urutan persis dari provider
	Budget int                      // TextBudget(cfg); <=0 = tanpa batas
}

// NewEndpointPage: bila !resp.SpecFound mengembalikan SpecMiss.
func NewEndpointPage(f Filter, offset int, resp listendpoints.Response, budget int) (EndpointPage, *SpecMiss) {
	if !resp.SpecFound {
		return EndpointPage{}, &SpecMiss{Tool: ToolListAPIEndpoints, Requested: f.Spec, Required: false, Valid: resp.Specs}
	}
	items := make([]listendpoints.Endpoint, len(resp.Items))
	copy(items, resp.Items)
	return EndpointPage{Filter: f, Offset: offset, Total: resp.Total, Items: items, Budget: budget}, nil
}

func (p EndpointPage) specLabel() string {
	if p.Filter.Spec == "" {
		return "semua"
	}
	return p.Filter.Spec
}

func formatRow(e listendpoints.Endpoint) string {
	var b strings.Builder
	b.WriteString(e.Method)
	b.WriteString(" ")
	b.WriteString(Clip(OneLine(e.Path), pathMaxRunes))
	if s := Clip(OneLine(e.Summary), summaryMaxRunes); s != "" {
		b.WriteString(" — " + s)
	}
	if id := Clip(OneLine(e.OperationID), opIDMaxRunes); id != "" {
		b.WriteString(" [" + id + "]")
	}
	var tags []string
	for _, t := range e.Tags {
		if t = OneLine(t); t != "" {
			tags = append(tags, t)
		}
	}
	if len(tags) > 0 {
		b.WriteString(" (" + Clip(strings.Join(tags, ", "), tagsMaxRunes) + ")")
	}
	if e.Deprecated {
		b.WriteString(" [deprecated]")
	}
	return b.String()
}

func (p EndpointPage) header(shown int) string {
	h := fmt.Sprintf("Endpoint %d-%d dari %d | spec: %s", p.Offset+1, p.Offset+shown, p.Total, p.specLabel())
	if p.Filter.Tag != "" {
		h += " | tag: " + p.Filter.Tag
	}
	if p.Filter.Query != "" {
		h += fmt.Sprintf(" | query: %q", p.Filter.Query)
	}
	return h
}

// render menghasilkan badan, jumlah baris yang muat, dan jumlah grup.
func (p EndpointPage) render() (body string, shown, groups int) {
	var b strings.Builder
	used := len(p.header(len(p.Items))) + 1
	prev := ""
	for i, it := range p.Items {
		newGroup := i == 0 || it.Spec != prev
		piece := formatRow(it) + "\n"
		if newGroup {
			piece = "== " + it.Spec + " ==\n" + piece
		}
		if i > 0 && p.Budget > 0 && used+len(piece)+closingReserve > p.Budget {
			break
		}
		b.WriteString(piece)
		used += len(piece)
		shown++
		if newGroup {
			groups++
		}
		prev = it.Spec
	}
	return strings.TrimSuffix(b.String(), "\n"), shown, groups
}

// Lines adalah badan: grup + baris yang muat Budget.
func (p EndpointPage) Lines() (string, int) {
	body, shown, _ := p.render()
	return body, shown
}

// NextOffset adalah Offset+shown bila masih ada sisa, selain itu -1.
func (p EndpointPage) NextOffset() int {
	_, shown, _ := p.render()
	if p.Offset+shown < p.Total {
		return p.Offset + shown
	}
	return -1
}

func (p EndpointPage) noFilter() bool {
	return p.Filter.Spec == "" && p.Filter.Tag == "" && p.Filter.Query == ""
}

func (p EndpointPage) emptyText() string {
	if p.noFilter() {
		return NoSpecsText
	}
	desc := "spec: " + p.specLabel()
	if p.Filter.Tag != "" {
		desc += ", tag: " + p.Filter.Tag
	}
	if p.Filter.Query != "" {
		desc += fmt.Sprintf(", query: %q", p.Filter.Query)
	}
	return "Tidak ada endpoint yang cocok (" + desc + "). Hapus filter tag/query, periksa nama tag dari list_api_endpoints tanpa filter, atau lihat spec dengan list_api_specs."
}

func (p EndpointPage) outOfRange() bool { return p.Total > 0 && len(p.Items) == 0 }

// Text adalah teks hasil tool.
func (p EndpointPage) Text() string {
	switch {
	case p.Total == 0:
		return p.emptyText()
	case p.outOfRange():
		return fmt.Sprintf("Argumen tidak valid: offset %d melewati total %d endpoint. Gunakan offset 0 sampai %d.", p.Offset, p.Total, p.Total-1)
	}
	body, shown, _ := p.render()
	var b strings.Builder
	b.WriteString(p.header(shown))
	b.WriteString("\n")
	b.WriteString(body)
	if next := p.NextOffset(); next >= 0 {
		fmt.Fprintf(&b, "\n\n[ada %d endpoint lagi, panggil list_api_endpoints dengan offset=%d dan argumen lain yang sama]\n", p.Total-p.Offset-shown, next)
	} else {
		b.WriteString("\n\n")
	}
	b.WriteString(endpointDetailHint)
	return b.String()
}

// Result adalah hasil tool; offset di luar rentang menjadi isError.
func (p EndpointPage) Result() toolkit.Result {
	res := toolkit.Result{Text: p.Text(), TargetType: TargetTypeSpec, Target: p.Filter.Spec}
	switch {
	case p.Total == 0:
		res.Summary = map[string]any{"count": 0, "total": 0, "next_offset": -1, "spec_groups": 0, "truncated": false}
	case p.outOfRange():
		res.IsError = true
		res.Summary = map[string]any{"error_code": "offset_out_of_range", "total": p.Total}
	default:
		_, shown, groups := p.render()
		next := p.NextOffset()
		res.Summary = map[string]any{"count": shown, "total": p.Total, "next_offset": next, "spec_groups": groups, "truncated": next >= 0}
	}
	return res
}
