package finddevelopers

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus core.auth.find-developers. Tidak ada route HTTP.
func Init(d app.Deps, _ base.Shared) {
	h := NewHandler(NewUsecase(NewRepository(d.DB)))
	d.Bus.Register(constants.BUS_CORE_AUTH_FIND_DEVELOPERS, h.Handle)
}
