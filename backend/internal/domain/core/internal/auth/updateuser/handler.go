package updateuser

import (
	"net/http"

	httpx "grimoire/internal/infra/http"
)

// Handler melayani PATCH /api/users/{id}.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Patch mengubah name/email/role user.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	p, err := parseParams(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := h.uc.UpdateUser(r.Context(), p)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res.User.ToResponse())
}
