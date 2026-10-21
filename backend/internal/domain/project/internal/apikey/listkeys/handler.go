package listkeys

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/http/respond"
)

type handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, res)
}
