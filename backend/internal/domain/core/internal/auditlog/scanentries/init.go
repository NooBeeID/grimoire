package scanentries

import (
	"time"

	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init merakit repo -> usecase -> handler dan mendaftarkan endpoint bus baca-saja.
func Init(d app.Deps) {
	uc := NewUsecase(NewRepository(d.DB), time.Now)
	d.Bus.Register(constants.BUS_CORE_AUDIT_SCAN_ENTRIES, handler{uc: uc}.Handle)
}
