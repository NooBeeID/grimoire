package getuser

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	httpx "grimoire/internal/infra/http"
)

// Handler melayani GET /api/users/{id}.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Get meneruskan id mentah ke usecase; validasi uuid ada di store.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	dto, err := h.uc.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, dto)
}
