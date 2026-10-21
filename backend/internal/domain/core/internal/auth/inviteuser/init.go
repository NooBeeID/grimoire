package inviteuser

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/domain/core/internal/auth/base/delivery"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/constants"
)

// Init mendaftarkan POST /users/invite (di bawah /api, khusus super_admin) dan handler bus
// core.auth.invite-user. Dipanggil sekali (bus Register ganda panic).
func Init(d app.Deps, s base.Shared) {
	send := func(ctx context.Context, to string, c base.EmailContent) string {
		return delivery.Deliver(ctx, d.Mailer, delivery.Message(to, c), d.Log)
	}
	uc := NewUsecase(NewRepo(d.DB), s.Hasher, audit.New(d.Bus, d.Log), s.Clock, s.Rand, s.Invitation, s.LoginURL, send).
		WithLogger(d.Log)
	d.API.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin)).
		Post("/users/invite", NewHandler(uc).Invite)
	d.Bus.Register(constants.BUS_CORE_AUTH_INVITE_USER, NewBusHandler(uc).Handle)
}
