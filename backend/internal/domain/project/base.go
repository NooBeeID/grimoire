package project

import (
	"grimoire/internal/domain/project/internal/apikey"
	"grimoire/internal/domain/project/internal/apispec"
	"grimoire/internal/domain/project/internal/gitsync"
	"grimoire/internal/domain/project/internal/knowledge"
	"grimoire/internal/domain/project/internal/management"
	"grimoire/internal/domain/project/internal/searchsynonym"
	"grimoire/internal/infra/app"
)

// Register merakit module domain ini.
func Register(d app.Deps) {
	management.Register(d)
	apikey.Register(d)
	knowledge.Register(d)
	searchsynonym.Register(d)
	apispec.Register(d)
	gitsync.Register(d)
}
