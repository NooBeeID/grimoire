package bussearch

import (
	"grimoire/internal/domain/project/internal/knowledge/base/searchrun"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.knowledge.search. Panggil sekali saja.
func Init(d app.Deps) {
	run := searchrun.New(store.New(d.DB), searchrun.NewBusSynonyms(d.Bus), d.Log)
	h := &Handler{uc: NewUsecase(run)}
	d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_SEARCH, h.Handle)
}
