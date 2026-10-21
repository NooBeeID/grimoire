// Package readfile adalah tool MCP read_file (T08).
package readfile

import (
	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Init merakit tool read_file. Config dibaca sekali di sini.
func Init(d app.Deps) toolkit.Tool {
	maxLimit := base.MaxReadLimit(d.Config.MCPMaxResponseBytes)
	uc := newUsecase(newRepository(d.Bus))
	return toolkit.Tool{
		Name:        base.ToolReadFile,
		Title:       "Baca file knowledge",
		Description: "Baca isi satu file knowledge (read-only). path persis seperti dari list_files atau search_knowledge. File besar dikembalikan per potongan: bila ada penanda [terpotong: ... offset=N ...], panggil lagi dengan offset=N untuk lanjutan tanpa duplikat. offset dan limit dalam byte.",
		InputSchema: schema(maxLimit),
		Handler:     newHandler(uc, maxLimit),
	}
}
