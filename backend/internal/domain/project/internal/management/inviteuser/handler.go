package inviteuser

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

const maxUserAgentRunes = 512

type executor interface {
	Execute(ctx context.Context, projectID string, in InviteRequest, meta base.RequestMeta) (InviteResponse, error)
}

type Handler struct{ uc executor }

func newHandler(uc executor) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in InviteRequest
	if err := respond.DecodeJSON(r, &in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	if err := in.Validate(); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), maxUserAgentRunes)}
	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), in, meta)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	if res.Credentials != nil {
		w.Header().Set("Cache-Control", "no-store")
	}
	respond.WriteJSON(w, http.StatusCreated, res)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}
