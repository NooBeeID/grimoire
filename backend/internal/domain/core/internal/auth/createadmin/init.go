package createadmin

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus core.auth.create-admin.
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(newRepo(d.DB), s.Hasher, audit.New(d.Bus, d.Log), s.Password)
	d.Bus.Register(constants.BUS_CORE_AUTH_CREATE_ADMIN, (&handler{uc: uc}).handle)
}
