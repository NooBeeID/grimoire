// Package endpoint melayani transport MCP Streamable HTTP (POST/GET/DELETE /mcp).
package endpoint

import (
	"net/http"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base/auditwriter"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
)

// Init mendaftarkan route /mcp.
func Init(d app.Deps, cat base.Catalog, inv base.ToolInvoker, admit func(http.Handler) http.Handler, audit auditwriter.Writer) {
	cfg := d.Config
	uc := NewUsecase(d.Log, d.Version, cat, inv, audit)
	h := &Handler{log: d.Log, uc: uc, maxBytes: cfg.MCPMaxRequestBytes}
	if h.log == nil {
		h.log = uc.log
	}

	d.MCP.With(
		middleware.OriginGuard(cfg.MCPAllowedOrigins),
		middleware.BodyLimit(cfg.MCPMaxRequestBytes),
		admit,
	).Post("/", h.Post)
	d.MCP.Get("/", h.MethodNotAllowed)
	d.MCP.Delete("/", h.MethodNotAllowed)
}
