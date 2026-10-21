package touchlastused

import (
	"time"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler Publish project.apikey.touch.
func Init(d app.Deps) {
	clock := base.Clock(func() time.Time { return time.Now().UTC() })
	uc := NewUsecase(newRepo(store.New(d.DB)), clock, d.Log)
	d.Bus.Register(constants.BUS_PROJECT_APIKEY_TOUCH, newHandler(uc).Handle)
}
