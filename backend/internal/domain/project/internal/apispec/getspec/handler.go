package getspec

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/http/respond"
)

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

type response struct {
	Spec      base.SpecDTO       `json:"spec"`
	Endpoints []base.EndpointDTO `json:"endpoints"`
	Document  json.RawMessage    `json:"document"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), chi.URLParam(r, "name"))
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, response{
		Spec:      base.ToSummaryDTO(res.Spec),
		Endpoints: base.ToEndpointDTOs(res.Endpoints),
		Document:  res.Document,
	})
}
