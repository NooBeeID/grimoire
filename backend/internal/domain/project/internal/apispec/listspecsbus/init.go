package listspecsbus

import (
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.apispec.list-specs. Tidak ada route HTTP.
func Init(d app.Deps) {
	uc := NewUsecase(store.New(d.DB))
	d.Bus.Register(constants.BUS_PROJECT_APISPEC_LIST_SPECS, newHandler(uc).Handle)
}
