// Package knowledgetools menyediakan tool MCP knowledge (T08) yang didaftarkan ke gateway T07 lewat toolkit.
package knowledgetools

import (
	"grimoire/internal/domain/discovery/internal/knowledgetools/listfiles"
	"grimoire/internal/domain/discovery/internal/knowledgetools/readfile"
	"grimoire/internal/domain/discovery/internal/knowledgetools/searchknowledge"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Register membangun tiga tool read-only. Tidak mendaftarkan endpoint bus dan tidak punya state.
func Register(d app.Deps) []toolkit.Tool {
	return []toolkit.Tool{listfiles.Init(d), readfile.Init(d), searchknowledge.Init(d)}
}
