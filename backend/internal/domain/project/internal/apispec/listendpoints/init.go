package listendpoints

import (
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.apispec.list-endpoints. Tidak ada route HTTP.
func Init(d app.Deps) {
	uc := NewUsecase(newRepo(d.DB, store.New(d.DB)))
	d.Bus.Register(constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, newHandler(uc).Handle)
}
