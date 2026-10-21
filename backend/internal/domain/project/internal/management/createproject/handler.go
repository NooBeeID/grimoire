package createproject

import (
	"net/http"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

const maxUserAgentRunes = 512

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in CreateRequest
	if err := respond.DecodeJSON(r, &in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), maxUserAgentRunes)}
	res, err := h.uc.Execute(r.Context(), in, meta)
	if err != nil {
		respond.WriteError(w, r, err)
		return
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
