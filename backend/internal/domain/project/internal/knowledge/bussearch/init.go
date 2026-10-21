package bussearch

import (
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.knowledge.search. Panggil sekali saja.
func Init(d app.Deps) {
	h := &Handler{uc: NewUsecase(store.New(d.DB))}
	d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_SEARCH, h.Handle)
}
