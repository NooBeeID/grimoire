package deletecredential

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type usecase interface {
	Execute(ctx context.Context, in Input) error
}

type handler struct{ uc usecase }

func newHandler(uc usecase) *handler { return &handler{uc: uc} }

// ServeHTTP melayani DELETE /{provider}; body tidak dibaca.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	in := Input{
		Provider: chi.URLParam(r, "provider"),
		Host:     r.URL.Query().Get("host"),
		Meta:     base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent()),
	}
	if err := h.uc.Execute(r.Context(), in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
