package buslistchanges

import (
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.knowledge.list-changes. Panggil sekali saja.
func Init(d app.Deps) {
	h := &Handler{uc: New(NewRepo(store.New(d.DB)))}
	d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_CHANGES, h.Handle)
}
