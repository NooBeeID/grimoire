// Package listapispecs adalah tool MCP api-spec-tools (T09): list_api_specs.
package listapispecs

import (
	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const (
	title       = "Daftar API spec"
	description = "Titik awal menjelajah API project (read-only): daftar spec OpenAPI beserta judul, versi API, versi OpenAPI, jumlah endpoint, dan waktu diperbarui. Kolom name dipakai sebagai argumen spec pada list_api_endpoints dan get_api_endpoint."
)

// Init membangun tool list_api_specs.
func Init(d app.Deps) toolkit.Tool {
	budget := base.TextBudget(d.Config.MCPMaxResponseBytes)
	h := newHandler(newUsecase(newRepository(d.Bus), budget))
	return toolkit.Tool{
		Name:        base.ToolListAPISpecs,
		Title:       title,
		Description: description,
		InputSchema: schema(),
		Handler:     h,
	}
}
