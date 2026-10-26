// Package searchsynonym merakit module project/searchsynonym: route HTTP dan handler bus kamus sinonim.
package searchsynonym

import (
	"grimoire/internal/domain/project/internal/searchsynonym/buslistgroups"
	"grimoire/internal/domain/project/internal/searchsynonym/creategroup"
	"grimoire/internal/domain/project/internal/searchsynonym/deletegroup"
	"grimoire/internal/domain/project/internal/searchsynonym/listgroups"
	"grimoire/internal/domain/project/internal/searchsynonym/updategroup"
	"grimoire/internal/infra/app"
)

// Register merakit module: handler bus dan route REST. Registrasi ganda = panic.
func Register(d app.Deps) {
	// Handler bus.
	buslistgroups.Init(d)

	// Route REST /api/projects/{projectID}/search/synonyms...
	listgroups.Init(d)
	creategroup.Init(d)
	updategroup.Init(d)
	deletegroup.Init(d)
}
