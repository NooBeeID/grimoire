package deletefile

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type executor interface {
	Execute(ctx context.Context, p params, meta base.RequestMeta) error
}

type handler struct{ uc executor }

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	p := params{
		ProjectID: chi.URLParam(r, "projectID"),
		Path:      r.URL.Query().Get("path"),
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	if err := h.uc.Execute(r.Context(), p, meta); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
