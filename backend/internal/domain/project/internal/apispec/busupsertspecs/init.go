package busupsertspecs

import (
	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.apispec.upsert-specs. Tidak ada route HTTP dan tidak ada
// pemeriksaan keanggotaan: consumer wajib check-access.
func Init(d app.Deps) {
	s := store.New(d.DB)
	clock := base.Clock(base.SystemClock)
	uc := NewUsecase(newRepo(d.DB, s), audit.New(d.Bus, d.Log, clock), clock,
		d.Config.APISpecMaxSpecs, d.Config.APISpecMaxFileBytes)
	d.Bus.Register(constants.BUS_PROJECT_APISPEC_UPSERT_SPECS, newHandler(uc).Handle)
}
