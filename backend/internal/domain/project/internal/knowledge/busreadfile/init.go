package busreadfile

import (
	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.knowledge.read-file. Panggil sekali saja.
func Init(d app.Deps) {
	uc := NewUsecase(store.New(d.DB), base.LimitsFrom(d.Config))
	d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, NewHandler(uc).Handle)
}
