package listspecs

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/http/respond"
)

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

type listResponse struct {
	Items []base.SpecDTO `json:"items"`
	Total int            `json:"total"`
	Limit int            `json:"limit"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	items := make([]base.SpecDTO, 0, len(res.Items))
	for _, s := range res.Items {
		items = append(items, base.ToSummaryDTO(s))
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, listResponse{Items: items, Total: len(items), Limit: res.Limit})
}
