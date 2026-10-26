// Package readsection adalah tool MCP read_section (T18).
package readsection

import (
	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Init merakit tool read_section. Config dibaca sekali di sini.
func Init(d app.Deps) toolkit.Tool {
	maxLimit := base.MaxReadLimit(d.Config.MCPMaxResponseBytes)
	uc := newUsecase(newRepository(d.Bus))
	return toolkit.Tool{
		Name:        base.ToolReadSection,
		Title:       "Baca satu bagian file knowledge",
		Description: "Baca satu bagian file Markdown knowledge (read-only), dari heading sampai sebelum heading selevel atau lebih tinggi. heading adalah teks persis dari get_file_outline (tanpa #); bila ada beberapa heading bernama sama, pakai occurrence. limit dalam byte; bagian yang terpotong diberi penanda read_file offset untuk lanjutan.",
		InputSchema: schema(maxLimit),
		Handler:     newHandler(uc, maxLimit),
	}
}
