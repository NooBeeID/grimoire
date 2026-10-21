package search

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/http/respond"
)

type executor interface {
	Execute(ctx context.Context, in Input) (base.SearchResultDTO, error)
}

type Handler struct{ uc executor }

func newHandler(uc executor) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	res, err := h.uc.Execute(r.Context(), Input{
		ProjectID: chi.URLParam(r, "projectID"),
		Query:     qs.Get("q"),
		Prefix:    qs.Get("prefix"),
		Limit:     qs.Get("limit"),
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, res)
}
