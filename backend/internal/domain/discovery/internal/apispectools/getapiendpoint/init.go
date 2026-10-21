// Package getapiendpoint adalah tool MCP api-spec-tools (T09): get_api_endpoint.
package getapiendpoint

import (
	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const toolDescription = "Detail satu operasi API (read-only) dalam JSON: parameter, request body, response, dengan skema $ref yang dibuka sampai schema_depth, servers, dan securitySchemes yang dipakai. Pilih operasi dengan method + path (path persis seperti di list_api_endpoints, termasuk {param}) ATAU operation_id. spec wajib bila project punya lebih dari satu spec. {\"$circular\": ref} = skema merujuk dirinya; {\"$ref\": ref} = tidak dibuka karena batas kedalaman. Bila endpoint tidak ketemu, gunakan saran Similar atau list_api_endpoints."

// Init membangun tool get_api_endpoint. Config dibaca sekali di sini.
func Init(d app.Deps) toolkit.Tool {
	maxBytes := base.ProviderMaxBytes(d.Config.MCPMaxResponseBytes)
	uc := newUsecase(newRepository(d.Bus), maxBytes)
	return toolkit.Tool{
		Name:        base.ToolGetAPIEndpoint,
		Title:       "Detail endpoint API",
		Description: toolDescription,
		InputSchema: schema(),
		Handler:     newHandler(uc),
	}
}
