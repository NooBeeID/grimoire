// Package searchknowledge adalah tool MCP search_knowledge (T08).
package searchknowledge

import (
	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const (
	title       = "Cari knowledge"
	description = "Cari file knowledge berdasarkan kata kunci (full-text, read-only, bukan semantik). Toleran terhadap sinonim, salah ketik, dan imbuhan kata. Frasa dalam tanda kutip dicari utuh. Hasil berisi path, skor, cuplikan dengan kata kunci bertanda **...**, dan field section (heading bagian cuplikan) yang bisa dipakai untuk read_section(path, heading); baca file utuh dengan read_file(path). Persempit dengan path=<folder>."
)

// Init merakit tool search_knowledge: repository -> usecase -> handler.
func Init(d app.Deps) toolkit.Tool {
	uc := newUsecase(newRepository(d.Bus))
	return toolkit.Tool{
		Name:        base.ToolSearchKnowledge,
		Title:       title,
		Description: description,
		InputSchema: schema(),
		Handler:     newHandler(uc),
	}
}
