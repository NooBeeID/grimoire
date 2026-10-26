// Package knowledgetools menyediakan tool MCP knowledge (T08) yang didaftarkan ke gateway T07 lewat toolkit,
// serta endpoint bus laporan kualitas pencarian (T11).
package knowledgetools

import (
	"grimoire/internal/domain/discovery/internal/knowledgetools/getfileoutline"
	"grimoire/internal/domain/discovery/internal/knowledgetools/listfiles"
	"grimoire/internal/domain/discovery/internal/knowledgetools/listrecentchanges"
	"grimoire/internal/domain/discovery/internal/knowledgetools/readfile"
	"grimoire/internal/domain/discovery/internal/knowledgetools/readsection"
	"grimoire/internal/domain/discovery/internal/knowledgetools/searchknowledge"
	"grimoire/internal/domain/discovery/internal/knowledgetools/searchquality"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Register membangun enam tool read-only dan mendaftarkan satu endpoint bus laporan kualitas pencarian.
// Tidak punya state; searchquality bukan tool sehingga daftar tool tidak berubah.
func Register(d app.Deps) []toolkit.Tool {
	searchquality.Init(d)
	return []toolkit.Tool{
		getfileoutline.Init(d), listfiles.Init(d), listrecentchanges.Init(d),
		readfile.Init(d), readsection.Init(d), searchknowledge.Init(d),
	}
}
