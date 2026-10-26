package listgroups

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/infra/http/respond"
)

type executor interface {
	Execute(ctx context.Context, projectID string) ([]base.GroupDTO, error)
}

type handler struct{ uc executor }

type response struct {
	Items []base.GroupDTO `json:"items"`
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	items, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	if items == nil {
		items = []base.GroupDTO{}
	}
	respond.WriteJSON(w, http.StatusOK, response{Items: items})
}
