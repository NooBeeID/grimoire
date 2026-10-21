package removemember

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
	Execute(ctx context.Context, in Input) error
}

type handler struct{ uc executor }

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h.uc.Execute(r.Context(), Input{
		ProjectID: chi.URLParam(r, "projectID"),
		UserID:    chi.URLParam(r, "userID"),
		Meta:      base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), maxUserAgentRunes)},
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
