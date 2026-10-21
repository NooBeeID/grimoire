package createfile

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type executor interface {
	Execute(ctx context.Context, projectID string, req CreateRequest, meta base.RequestMeta) (base.FileDTO, error)
}

type Handler struct {
	uc  executor
	log *slog.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := respond.DecodeJSON(r, &req); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	dto, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), req, meta)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusCreated, dto)
}
