package getfile

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/http/respond"
)

type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	dto, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), r.URL.Query().Get("path"))
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, dto)
}
