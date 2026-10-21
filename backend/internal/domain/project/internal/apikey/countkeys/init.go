package countkeys

import (
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.apikey.count-keys.
func Init(d app.Deps) {
	uc := NewUsecase(newRepo(store.New(d.DB)))
	d.Bus.Register(constants.BUS_PROJECT_APIKEY_COUNT_KEYS, newHandler(uc).Handle)
}
