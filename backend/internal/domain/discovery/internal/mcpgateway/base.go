// Package mcpgateway adalah module gateway MCP (server Streamable HTTP stateless di /mcp).
package mcpgateway

import (
	"grimoire/internal/domain/discovery/internal/mcpgateway/authenticate"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base/auditwriter"
	"grimoire/internal/domain/discovery/internal/mcpgateway/callaudit"
	"grimoire/internal/domain/discovery/internal/mcpgateway/endpoint"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Register memasang gateway MCP di d.MCP. tools berasal dari domain/discovery/base.go.
// Katalog tidak valid (nama duplikat dll.) -> panic saat start.
func Register(d app.Deps, tools []toolkit.Tool) {
	cat, err := base.NewCatalog(tools)
	if err != nil {
		panic("mcpgateway: " + err.Error())
	}
	audit := auditwriter.New(d.Bus, d.Log)
	inv := callaudit.Init(d, cat, audit)
	admit := authenticate.Init(d, audit)
	endpoint.Init(d, cat, inv, admit, audit)
}
