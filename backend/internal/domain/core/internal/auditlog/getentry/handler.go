package getentry

import (
	"net/http"

	"grimoire/internal/infra/http/respond"
)

// Handler adalah adapter HTTP untuk GET /api/audit-log/{id}.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Get melayani GET /{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := parseParams(r)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	e, err := h.uc.Get(r.Context(), p)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, e.ToDetail())
}
