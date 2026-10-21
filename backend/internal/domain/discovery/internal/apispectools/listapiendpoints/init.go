// Package listapiendpoints adalah tool MCP api-spec-tools (T09): list_api_endpoints.
package listapiendpoints

import (
	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const (
	title       = "Daftar endpoint API"
	description = "Daftar endpoint API (read-only), satu baris per operasi: METHOD /path — ringkasan [operationId] (tag). Tanpa spec = semua spec, dikelompokkan per spec. Saring dengan tag (persis) atau query (cocok sebagian pada path, ringkasan, operationId). Lanjutkan daftar terpotong dengan offset. Ambil detail dengan get_api_endpoint."
)

// Init merakit tool list_api_endpoints: repository -> usecase -> handler.
func Init(d app.Deps) toolkit.Tool {
	budget := base.TextBudget(d.Config.MCPMaxResponseBytes)
	return toolkit.Tool{
		Name:        base.ToolListAPIEndpoints,
		Title:       title,
		Description: description,
		InputSchema: schema(),
		Handler:     newHandler(newUsecase(newRepository(d.Bus), budget)),
	}
}
