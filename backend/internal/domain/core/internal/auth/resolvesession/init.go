package resolvesession

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus core.auth.resolve-session.
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(NewRepo(d.DB), s.Clock, s.Session, d.Log)
	d.Bus.Register(constants.BUS_CORE_AUTH_RESOLVE_SESSION, Handle(uc))
}
