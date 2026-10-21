package getproject

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
)

type executor interface {
	Execute(ctx context.Context, projectID string) (base.ProjectDetail, error)
}

type Handler struct{ uc executor }

func newHandler(uc executor) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, d)
}
