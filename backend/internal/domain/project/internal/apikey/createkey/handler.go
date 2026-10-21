package createkey

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	var in CreateRequest
	if err := respond.DecodeJSON(r, &in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), in, meta)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusCreated, res)
}
