package verifykey

import (
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

func Init(d app.Deps) {
	uc := NewUsecase(newRepo(store.New(d.DB)))
	d.Bus.Register(constants.BUS_PROJECT_APIKEY_VERIFY, newHandler(uc).Handle)
}
