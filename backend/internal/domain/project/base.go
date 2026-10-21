package project

import (
	"grimoire/internal/domain/project/internal/management"
	"grimoire/internal/infra/app"
)

// Register merakit module domain ini.
func Register(d app.Deps) { management.Register(d) }
