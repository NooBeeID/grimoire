package core

import (
	"grimoire/internal/domain/core/internal/auditlog"
	"grimoire/internal/infra/app"
)

// Register merakit module domain ini. T02 (audit-log) terpasang; T01 (auth) menyusul.
func Register(d app.Deps) {
	auditlog.Register(d)
}
