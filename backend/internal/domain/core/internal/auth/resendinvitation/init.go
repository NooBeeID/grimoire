package resendinvitation

import (
	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/constants"
)

// Init mendaftarkan POST /users/{id}/resend-invitation (di bawah /api, khusus super_admin)
// dan handler bus core.auth.resend-invitation. Dipanggil sekali (bus Register ganda panic).
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(NewRepo(d.DB), s.Hasher, d.Mailer, audit.New(d.Bus, d.Log), s.Clock, s.Rand,
		s.Invitation, s.LoginURL, d.Config.MailTimeout, d.Log)
	d.API.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin)).
		Post("/users/{id}/resend-invitation", NewHandler(uc).Resend)
	d.Bus.Register(constants.BUS_CORE_AUTH_RESEND_INVITATION, NewBusHandler(uc).Handle)
}
