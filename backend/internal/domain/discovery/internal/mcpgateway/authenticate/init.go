// Package authenticate adalah middleware admisi POST /mcp: API key + rate limit.
package authenticate

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base/auditwriter"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
)

// Init merakit middleware admisi (key limiter, limiter IP gagal-auth, throttle audit).
func Init(d app.Deps, audit auditwriter.Writer) func(http.Handler) http.Handler {
	cfg := d.Config
	keyLim := middleware.NewLimiter(d.Ctx, middleware.Rule{
		Name: "mcp-key", Rate: rate.Limit(float64(cfg.MCPRatePerMinute) / 60), Burst: cfg.MCPRatePerMinute,
		KeyFunc: func(r *http.Request) string { return reqctx.APIKeyID(r.Context()) },
	})
	ipLim := middleware.NewLimiter(d.Ctx, middleware.Rule{
		Name: "mcp-auth-fail-ip", Rate: rate.Limit(float64(cfg.MCPAuthFailPerMinute) / 60), Burst: cfg.MCPAuthFailPerMinute,
		KeyFunc: middleware.KeyByIP,
	})
	th := base.NewAuditThrottle(time.Minute, time.Now)
	uc := NewUsecase(d.Bus, ipLim, keyLim, th, audit, d.Log)
	return newHandler(uc, d.Log).middleware
}
