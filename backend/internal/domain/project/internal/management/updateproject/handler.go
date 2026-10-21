package updateproject

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

const maxUserAgentRunes = 512

type handler struct{ uc *Usecase }

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req UpdateRequest
	if err := respond.DecodeJSON(r, &req); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	out, err := h.uc.Execute(r.Context(), Input{
		ProjectID: chi.URLParam(r, "projectID"),
		Patch:     req.Patch(),
		Meta:      base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), maxUserAgentRunes)},
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, out)
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
