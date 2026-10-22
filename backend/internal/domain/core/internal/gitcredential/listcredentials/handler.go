package listcredentials

import (
	"net/http"

	"grimoire/internal/infra/http/respond"
)

type handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h.uc.Execute(r.Context())
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, res)
}
