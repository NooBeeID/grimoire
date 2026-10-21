// Package auditlog adalah module core/audit-log (T02): pencatatan dan peninjauan audit.
package auditlog

import (
	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auditlog/getentry"
	"grimoire/internal/domain/core/internal/auditlog/listentries"
	"grimoire/internal/domain/core/internal/auditlog/purgeexpired"
	"grimoire/internal/domain/core/internal/auditlog/record"
	"grimoire/internal/domain/core/internal/auditlog/scanentries"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

// Register merakit module audit-log. Hanya super_admin yang boleh membaca log.
// Juga mendaftarkan endpoint bus baca-saja core.audit.scan-entries untuk consumer lintas module (T11).
func Register(d app.Deps) {
	record.Init(d)
	scanentries.Init(d)
	d.API.Route("/audit-log", func(r chi.Router) {
		r.Use(middleware.RequireRole(resolvesession.RoleSuperAdmin))
		listentries.Init(d, r)
		getentry.Init(d, r)
	})
	purgeexpired.Init(d)
}
