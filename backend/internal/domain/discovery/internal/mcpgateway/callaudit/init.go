// Package callaudit mengeksekusi tools/call: lookup, timeout, recover, truncation, audit.
package callaudit

import (
	"time"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base/auditwriter"
	"grimoire/internal/infra/app"
)

// Init merakit base.ToolInvoker dari config.
func Init(d app.Deps, cat base.Catalog, audit auditwriter.Writer) base.ToolInvoker {
	return NewUsecase(cat, audit, d.Log, Options{
		Timeout:  d.Config.MCPToolTimeout,
		MaxBytes: d.Config.MCPMaxResponseBytes,
		Now:      time.Now,
	})
}
