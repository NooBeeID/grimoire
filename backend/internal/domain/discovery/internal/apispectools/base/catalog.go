package base

import (
	"fmt"
	"strings"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

const (
	specNameMaxRunes    = 80
	specTitleMaxRunes   = 120
	specVersionMaxRunes = 40

	catalogFooter = "Langkah berikut: list_api_endpoints(spec=\"<name>\") untuk daftar endpoint, lalu get_api_endpoint untuk detail."
)

// SpecCatalog adalah hasil list_api_specs. Murni, tanpa I/O.
type SpecCatalog struct {
	Items  []listspecs.Spec // urutan persis dari T06 (Name)
	Budget int              // batas byte teks, dari TextBudget(cfg); <=0 = tanpa batas
}

// NewSpecCatalog menyalin resp.Items (nil aman) dan budget.
func NewSpecCatalog(resp listspecs.Response, budget int) SpecCatalog {
	items := make([]listspecs.Spec, len(resp.Items))
	copy(items, resp.Items)
	return SpecCatalog{Items: items, Budget: budget}
}

// EndpointTotal adalah jumlah EndpointCount semua item (termasuk yang tidak tampil).
func (c SpecCatalog) EndpointTotal() int {
	n := 0
	for _, it := range c.Items {
		n += it.EndpointCount
	}
	return n
}

// Text adalah teks hasil tool.
func (c SpecCatalog) Text() string {
	t, _ := c.render()
	return t
}

// Result adalah hasil tool sukses (tidak pernah isError).
func (c SpecCatalog) Result() toolkit.Result {
	text, shown := c.render()
	return toolkit.Result{
		Text:       text,
		TargetType: TargetTypeSpec,
		Summary: map[string]any{
			"count":          len(c.Items),
			"endpoint_count": c.EndpointTotal(),
			"shown":          shown,
		},
	}
}

func (c SpecCatalog) specLine(s listspecs.Spec) string {
	orDash := func(v string) string {
		if v == "" {
			return "-"
		}
		return v
	}
	name := Clip(OneLine(s.Name), specNameMaxRunes)
	title := orDash(Clip(OneLine(s.Title), specTitleMaxRunes))
	av := orDash(Clip(OneLine(s.ApiVersion), specVersionMaxRunes))
	ov := SpecVersionLabel(Clip(OneLine(s.OpenapiVersion), specVersionMaxRunes))
	return fmt.Sprintf("- %s: %s | versi %s | %s | %d endpoint | diperbarui %s",
		name, title, av, ov, s.EndpointCount, FormatTime(s.UpdatedAt))
}

func hiddenMarker(rest int) string {
	return fmt.Sprintf("[%d spec lain tidak ditampilkan; gunakan list_api_endpoints(spec=...) dengan nama yang sudah diketahui]", rest)
}

func (c SpecCatalog) render() (string, int) {
	if len(c.Items) == 0 {
		return NoSpecsText, 0
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d API spec di project ini:", len(c.Items)))
	footer := "\n" + catalogFooter
	if c.Budget > 0 {
		// Bila seluruh daftar muat, tampilkan semua (tanpa cadangan penanda).
		total := b.Len() + len(footer)
		for _, it := range c.Items {
			total += 1 + len(c.specLine(it))
		}
		if total <= c.Budget {
			return c.renderAll(), len(c.Items)
		}
	}
	shown := 0
	for i, it := range c.Items {
		line := "\n" + c.specLine(it)
		rest := len(c.Items) - i - 1
		reserve := 0
		if rest > 0 {
			reserve = len("\n" + hiddenMarker(rest))
		}
		if c.Budget > 0 && b.Len()+len(line)+reserve+len(footer) > c.Budget {
			break
		}
		b.WriteString(line)
		shown++
	}
	if shown < len(c.Items) {
		b.WriteString("\n" + hiddenMarker(len(c.Items)-shown))
	}
	b.WriteString(footer)
	return b.String(), shown
}

func (c SpecCatalog) renderAll() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d API spec di project ini:", len(c.Items))
	for _, it := range c.Items {
		b.WriteString("\n" + c.specLine(it))
	}
	b.WriteString("\n" + catalogFooter)
	return b.String()
}
