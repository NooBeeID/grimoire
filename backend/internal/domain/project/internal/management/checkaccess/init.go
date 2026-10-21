package checkaccess

import (
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.management.check-access.
func Init(d app.Deps) {
	uc := NewUsecase(store.New(d.DB), users.New(d.Bus))
	d.Bus.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, newHandler(uc).Handle)
}
