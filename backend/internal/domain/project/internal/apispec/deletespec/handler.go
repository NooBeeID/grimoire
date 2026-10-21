package deletespec

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type executor interface {
	Execute(ctx context.Context, p Params, meta base.RequestMeta) error
}

type Handler struct{ uc executor }

func newHandler(uc executor) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := Params{ProjectID: chi.URLParam(r, "projectID"), Name: chi.URLParam(r, "name")}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	if err := h.uc.Execute(r.Context(), p, meta); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
