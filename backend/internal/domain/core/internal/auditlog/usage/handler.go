package usage

import (
	"context"
	"net/http"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/http/respond"
)

type usager interface {
	Usage(ctx context.Context, q base.UsageQuery) (base.UsageReport, error)
}

// Handler adalah adapter HTTP untuk GET /api/audit-log/usage.
type Handler struct{ uc usager }

// NewHandler membuat Handler.
func NewHandler(uc usager) *Handler { return &Handler{uc: uc} }

// Get melayani GET /usage.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	q := ParseQuery(r.URL.Query())
	if err := q.Validate(); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	rep, err := h.uc.Usage(r.Context(), q)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, rep)
}
