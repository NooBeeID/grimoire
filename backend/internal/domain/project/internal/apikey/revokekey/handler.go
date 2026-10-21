package revokekey

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type usecase interface {
	Execute(ctx context.Context, in Input) (base.KeyDTO, error)
}

type handler struct{ uc usecase }

func newHandler(uc usecase) *handler { return &handler{uc: uc} }

// ServeHTTP: tanpa request body (diabaikan, tidak di-decode).
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	in := Input{
		ProjectID: chi.URLParam(r, "projectID"),
		KeyID:     chi.URLParam(r, "keyID"),
		Meta:      base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent()),
	}
	dto, err := h.uc.Execute(r.Context(), in)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, base.KeyEnvelope{Key: dto})
}
