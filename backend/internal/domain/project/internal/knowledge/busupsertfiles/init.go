package busupsertfiles

import (
	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.knowledge.upsert-files. Panggil sekali saja.
// Sengaja tanpa route HTTP dan tanpa pemeriksaan akses: consumer wajib check-access.
func Init(d app.Deps) {
	limits := base.LimitsFrom(d.Config)
	uc := NewUsecase(NewRepo(store.New(d.DB)), audit.New(d.Bus, d.Log), limits)
	h := &Handler{uc: uc}
	d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, h.Handle)
}
