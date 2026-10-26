// Package knowledge merakit module project/knowledge: route HTTP dan handler bus.
package knowledge

import (
	"grimoire/internal/domain/project/internal/knowledge/buslistchanges"
	"grimoire/internal/domain/project/internal/knowledge/buslistfiles"
	"grimoire/internal/domain/project/internal/knowledge/busreadfile"
	"grimoire/internal/domain/project/internal/knowledge/bussearch"
	"grimoire/internal/domain/project/internal/knowledge/busupsertfiles"
	"grimoire/internal/domain/project/internal/knowledge/countfiles"
	"grimoire/internal/domain/project/internal/knowledge/createfile"
	"grimoire/internal/domain/project/internal/knowledge/deletefile"
	"grimoire/internal/domain/project/internal/knowledge/deletefolder"
	"grimoire/internal/domain/project/internal/knowledge/getfile"
	"grimoire/internal/domain/project/internal/knowledge/listtree"
	"grimoire/internal/domain/project/internal/knowledge/movefile"
	"grimoire/internal/domain/project/internal/knowledge/search"
	"grimoire/internal/domain/project/internal/knowledge/updatefile"
	"grimoire/internal/domain/project/internal/knowledge/uploadfiles"
	"grimoire/internal/infra/app"
)

// Register merakit module: handler bus dan route REST. Urutan bebas; registrasi ganda = panic.
func Register(d app.Deps) {
	// Handler bus.
	countfiles.Init(d)
	buslistfiles.Init(d)
	buslistchanges.Init(d)
	busreadfile.Init(d)
	bussearch.Init(d)
	busupsertfiles.Init(d)

	// Route REST /api/projects/{projectID}/knowledge/...
	uploadfiles.Init(d)
	createfile.Init(d)
	updatefile.Init(d)
	getfile.Init(d)
	listtree.Init(d)
	deletefile.Init(d)
	deletefolder.Init(d)
	movefile.Init(d)
	search.Init(d)
}
