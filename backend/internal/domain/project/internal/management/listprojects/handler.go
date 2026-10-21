package listprojects

import (
	"net/http"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
)

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	in, fe := parseQuery(r.URL.Query())
	if len(fe) > 0 {
		respond.WriteError(w, r, base.InvalidFields(fe))
		return
	}
	out, err := h.uc.Execute(r.Context(), in)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, out)
}
