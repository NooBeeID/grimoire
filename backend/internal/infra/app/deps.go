// Package app memuat Deps yang diteruskan ke wiring domain.
package app

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/db"
	"grimoire/internal/infra/jobs"
	"grimoire/internal/infra/mailer"
)

// Deps adalah dependensi bersama untuk wiring domain. Dirakit oleh
// cmd/internal/server; domain hanya membaca field ini.
type Deps struct {
	// Ctx hidup selama server melayani request; dibatalkan setelah HTTP drain.
	Ctx    context.Context
	Config config.Config
	Log    *slog.Logger
	DB     *db.DB
	Bus    *bus.Registry
	Jobs   *jobs.Scheduler
	// Mailer generik; nil tidak pernah terjadi (driver noop bila tidak dikonfigurasi).
	Mailer mailer.Mailer
	// API adalah sub-router /api; MCP adalah sub-router /mcp.
	API chi.Router
	MCP chi.Router
	// Version adalah versi build untuk serverInfo MCP; diisi cmd/internal/server dari buildinfo.Version.
	Version string
}
