package getendpoint

import (
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.apispec.get-endpoint. Tidak ada route HTTP.
func Init(d app.Deps) {
	uc := NewUsecase(newRepo(store.New(d.DB)), d.Log)
	d.Bus.Register(constants.BUS_PROJECT_APISPEC_GET_ENDPOINT, newHandler(uc).Handle)
}
