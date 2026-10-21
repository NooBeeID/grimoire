// Package apispectools menyediakan tool MCP API spec (T09) yang didaftarkan ke gateway T07 lewat toolkit.
package apispectools

import (
	"grimoire/internal/domain/discovery/internal/apispectools/getapiendpoint"
	"grimoire/internal/domain/discovery/internal/apispectools/listapiendpoints"
	"grimoire/internal/domain/discovery/internal/apispectools/listapispecs"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Register membangun tiga tool read-only. Tidak mendaftarkan endpoint bus dan tidak punya state.
func Register(d app.Deps) []toolkit.Tool {
	return []toolkit.Tool{listapispecs.Init(d), listapiendpoints.Init(d), getapiendpoint.Init(d)}
}
