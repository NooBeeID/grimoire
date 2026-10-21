package searchquality

import (
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus discovery.knowledgetools.search-quality-report.
// Dipanggil dari knowledgetools/base.go pada tahap wiring, bukan dari feature lain.
func Init(d app.Deps) {
	uc := NewUsecase(NewRepository(d.Bus))
	d.Bus.Register(constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, (&handler{uc: uc}).handle)
}
