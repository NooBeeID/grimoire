package movefile

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type executor interface {
	Execute(ctx context.Context, projectID string, in Request, meta base.RequestMeta) (base.FileDTO, error)
}

type Handler struct{ uc executor }

func NewHandler(uc executor) *Handler { return &Handler{uc: uc} }

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	var in Request
	if err := respond.DecodeJSON(r, &in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	dto, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), in, meta)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond.WriteJSON(w, http.StatusOK, dto)
}
