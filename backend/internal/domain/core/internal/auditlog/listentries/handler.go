package listentries

import (
	"context"
	"net/http"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/http/respond"
)

type lister interface {
	List(ctx context.Context, f base.Filter) (Result, error)
}

// Handler adalah adapter HTTP untuk GET /api/audit-log.
type Handler struct{ uc lister }

// NewHandler membuat Handler.
func NewHandler(uc lister) *Handler { return &Handler{uc: uc} }

// List melayani GET /.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, err := ParseFilter(r.URL.Query())
	if err == nil {
		err = f.Validate()
	}
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	res, err := h.uc.List(r.Context(), f)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, res)
}
