package getme

import (
	"net/http"

	"grimoire/internal/infra/http/respond"
)

// Handler melayani GET /api/auth/me.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Get menulis profil sesi sebagai JSON (query dan body diabaikan).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	me, err := h.uc.Execute(r.Context())
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, me)
}
