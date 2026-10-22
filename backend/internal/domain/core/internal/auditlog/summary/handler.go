package summary

import (
	"context"
	"net/http"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/http/respond"
)

type summarizer interface {
	Summarize(ctx context.Context, q base.SummaryQuery) (base.Report, error)
}

// Handler adalah adapter HTTP untuk GET /api/audit-log/summary.
type Handler struct{ uc summarizer }

// NewHandler membuat Handler.
func NewHandler(uc summarizer) *Handler { return &Handler{uc: uc} }

// Get melayani GET /summary.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	q := ParseQuery(r.URL.Query())
	if err := q.Validate(); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	rep, err := h.uc.Summarize(r.Context(), q)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, rep)
}
