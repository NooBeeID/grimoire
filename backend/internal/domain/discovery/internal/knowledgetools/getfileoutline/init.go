// Package getfileoutline adalah tool MCP get_file_outline (T18).
package getfileoutline

import (
	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Init merakit tool get_file_outline. Config dibaca sekali di sini.
func Init(d app.Deps) toolkit.Tool {
	budget := base.MaxReadLimit(d.Config.MCPMaxResponseBytes)
	uc := newUsecase(newRepository(d.Bus), budget)
	return toolkit.Tool{
		Name:        base.ToolGetFileOutline,
		Title:       "Kerangka file knowledge",
		Description: "Daftar heading satu file Markdown knowledge (read-only). Isi: level, teks, nomor baris, dan rentang byte tiap heading, beserta ukuran dan waktu diperbarui. Pakai ini sebelum read_file untuk file panjang, lalu baca satu bagian dengan read_section. File bukan Markdown mengembalikan outline kosong dengan ukurannya.",
		InputSchema: schema(),
		Handler:     newHandler(uc),
	}
}
