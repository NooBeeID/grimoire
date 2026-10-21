package listinvitations

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
)

type executor interface {
	Execute(ctx context.Context, projectID string) (Result, error)
}

type handler struct{ uc executor }

func newHandler(uc executor) *handler { return &handler{uc: uc} }

type response struct {
	Items []base.InvitationDTO `json:"items"`
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	items := res.Items
	if items == nil {
		items = []base.InvitationDTO{}
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, response{Items: items})
}
