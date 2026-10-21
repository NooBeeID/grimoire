package resendinvitation

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/http/respond"
)

type executor interface {
	Execute(ctx context.Context, in Input) (ResendResponse, error)
}

type handler struct{ uc executor }

// ServeHTTP tidak membaca body.
func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h.uc.Execute(r.Context(), Input{
		ProjectID: chi.URLParam(r, "projectID"),
		UserID:    chi.URLParam(r, "userID"),
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	if res.Credentials != nil {
		w.Header().Set("Cache-Control", "no-store")
	}
	respond.WriteJSON(w, http.StatusOK, res)
}
