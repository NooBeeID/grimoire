package countfiles

import (
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.knowledge.count-files. Panggil sekali saja.
func Init(d app.Deps) {
	uc := NewUsecase(NewRepository(store.New(d.DB)))
	d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, handle(uc))
}
