package summary

import (
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
)

// Init merakit repository -> usecase -> handler dan mendaftarkan GET /summary.
// RequireRole(super_admin) dipasang di grup /audit-log oleh auditlog/base.go.
// Tidak mendereferensi d.DB/d.Bus saat dirakit.
func Init(d app.Deps, r chi.Router) {
	uc := NewUsecase(NewRepository(d.DB), NewQualityReporter(d.Bus), time.Now, d.Log)
	r.Get("/summary", NewHandler(uc).Get)
}
