package core

import (
	"grimoire/internal/domain/core/internal/auditlog"
	"grimoire/internal/domain/core/internal/auth"
	"grimoire/internal/infra/app"
)

// Register merakit module domain ini. T03 (project) dan T07 (discovery) ada di domain masing-masing.
func Register(d app.Deps) {
	auditlog.Register(d)
	auth.Register(d)
}
