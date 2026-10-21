package listtree

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/http/respond"
)

type executor interface {
	Execute(ctx context.Context, in Input) (base.Level, error)
}

type Handler struct{ uc executor }

func newHandler(uc executor) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lvl, err := h.uc.Execute(r.Context(), Input{
		ProjectID: chi.URLParam(r, "projectID"),
		Prefix:    r.URL.Query().Get("prefix"),
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, lvl)
}
