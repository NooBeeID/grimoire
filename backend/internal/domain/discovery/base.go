package discovery

import (
	"grimoire/internal/domain/discovery/internal/apispectools"
	"grimoire/internal/domain/discovery/internal/knowledgetools"
	"grimoire/internal/domain/discovery/internal/mcpgateway"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Register merakit module domain discovery. Tool dari module lain ditambahkan di sini (T08, T09)
// tanpa gateway meng-import package tool. Urutan append tidak berpengaruh (katalog diurutkan nama).
func Register(d app.Deps) {
	var tools []toolkit.Tool
	tools = append(tools, knowledgetools.Register(d)...)
	tools = append(tools, apispectools.Register(d)...)
	mcpgateway.Register(d, tools)
}
