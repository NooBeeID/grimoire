package deletegroup

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type executor interface {
	Execute(ctx context.Context, projectID, groupID string, meta base.RequestMeta) error
}

type handler struct{ uc executor }

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	if err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), chi.URLParam(r, "groupID"), meta); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
