package updategroup

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type executor interface {
	Execute(ctx context.Context, projectID, groupID string, in Input, meta base.RequestMeta) (base.GroupDTO, error)
}

type handler struct{ uc executor }

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	var in Input
	if err := respond.DecodeJSON(r, &in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	dto, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), chi.URLParam(r, "groupID"), in, meta)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, dto)
}
