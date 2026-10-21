package getuserbus

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus core.auth.get-user.
func Init(d app.Deps, sh base.Shared) {
	h := NewHandler(NewUsecase(NewRepository(d.DB), sh.Clock))
	d.Bus.Register(constants.BUS_CORE_AUTH_GET_USER, h.Handle)
}
