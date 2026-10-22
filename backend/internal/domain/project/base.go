package project

import (
	"grimoire/internal/domain/project/internal/apikey"
	"grimoire/internal/domain/project/internal/apispec"
	"grimoire/internal/domain/project/internal/gitsync"
	"grimoire/internal/domain/project/internal/knowledge"
	"grimoire/internal/domain/project/internal/management"
	"grimoire/internal/infra/app"
)

// Register merakit module domain ini.
func Register(d app.Deps) {
	management.Register(d)
	apikey.Register(d)
	knowledge.Register(d)
	apispec.Register(d)
	gitsync.Register(d)
}
