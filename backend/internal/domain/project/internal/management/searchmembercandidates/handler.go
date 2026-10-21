package searchmembercandidates

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
)

type executor interface {
	Execute(ctx context.Context, in Input) ([]base.CandidateDTO, error)
}

type Handler struct{ uc executor }

func newHandler(uc executor) *Handler { return &Handler{uc: uc} }

type response struct {
	Items []base.CandidateDTO `json:"items"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	items, err := h.uc.Execute(r.Context(), Input{
		ProjectID: chi.URLParam(r, "projectID"),
		RawQ:      r.URL.Query().Get("q"),
		RawLimit:  r.URL.Query().Get("limit"),
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, response{Items: items})
}
