// Package listfiles adalah tool MCP knowledge-tools (T08): list_files.
package listfiles

import (
	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const (
	title       = "Daftar file knowledge"
	description = "Daftar file knowledge project (read-only). Mulai dari akar (tanpa path) untuk memahami struktur sebelum membaca. Default hanya satu tingkat: subfolder tampil sebagai entri [folder]; panggil lagi dengan path=<folder> untuk masuk, atau recursive=true untuk semua file di bawah path. Lanjutkan daftar yang terpotong dengan offset. Baca isi dengan read_file, cari berdasarkan isi dengan search_knowledge."
)

// Init membangun tool list_files.
func Init(d app.Deps) toolkit.Tool {
	return toolkit.Tool{
		Name:        base.ToolListFiles,
		Title:       title,
		Description: description,
		InputSchema: schema(),
		Handler:     newHandler(newUsecase(newRepository(d.Bus))),
	}
}
