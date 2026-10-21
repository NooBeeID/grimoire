package getprojectbus

import (
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.management.get-project.
func Init(d app.Deps) {
	uc := NewUsecase(store.New(d.DB))
	d.Bus.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, newHandler(uc).Handle)
}
