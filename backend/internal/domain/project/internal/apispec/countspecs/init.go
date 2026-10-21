package countspecs

import (
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.apispec.count-specs. Tidak ada route HTTP.
func Init(d app.Deps) {
	uc := NewUsecase(newRepo(d.DB))
	d.Bus.Register(constants.BUS_PROJECT_APISPEC_COUNT_SPECS, newHandler(uc).Handle)
}
