// Package apispec merakit module project/api-spec: route HTTP dan handler bus.
package apispec

import (
	"grimoire/internal/domain/project/internal/apispec/busupsertspecs"
	"grimoire/internal/domain/project/internal/apispec/countspecs"
	"grimoire/internal/domain/project/internal/apispec/deletespec"
	"grimoire/internal/domain/project/internal/apispec/downloadspec"
	"grimoire/internal/domain/project/internal/apispec/getendpoint"
	"grimoire/internal/domain/project/internal/apispec/getspec"
	"grimoire/internal/domain/project/internal/apispec/listendpoints"
	"grimoire/internal/domain/project/internal/apispec/listspecs"
	"grimoire/internal/domain/project/internal/apispec/listspecsbus"
	"grimoire/internal/domain/project/internal/apispec/renamespec"
	"grimoire/internal/domain/project/internal/apispec/uploadspecs"
	"grimoire/internal/domain/project/internal/apispec/validatespec"
	"grimoire/internal/infra/app"
)

// Register merakit module: handler bus dan route REST (pola penuh di d.API).
func Register(d app.Deps) {
	// Handler bus.
	countspecs.Init(d)
	listspecsbus.Init(d)
	listendpoints.Init(d)
	getendpoint.Init(d)
	busupsertspecs.Init(d)

	// Route REST /api/projects/{projectID}/api-specs...
	uploadspecs.Init(d)
	validatespec.Init(d)
	listspecs.Init(d)
	getspec.Init(d)
	downloadspec.Init(d)
	renamespec.Init(d)
	deletespec.Init(d)
}
