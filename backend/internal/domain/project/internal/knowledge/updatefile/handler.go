package updatefile

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
	Execute(ctx context.Context, in Input) (base.FileDTO, error)
}

type Handler struct {
	uc  executor
	log *slog.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req UpdateRequest
	if err := respond.DecodeJSON(r, &req); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	in := Input{
		ProjectID: chi.URLParam(r, "projectID"),
		Path:      r.URL.Query().Get("path"),
		Content:   req.Content,
		Version:   req.Version,
		Meta:      base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent()),
	}
	dto, err := h.uc.Execute(r.Context(), in)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, dto)
}
