package resetpassword

import (
	"bufio"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
)

// Handler melayani POST /api/users/{id}/reset-password.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

type peekedBody struct {
	io.Reader
	io.Closer
}

// bodyIsEmpty mengintip 1 byte; body dikembalikan utuh ke r.Body bila tidak kosong.
func bodyIsEmpty(r *http.Request) (bool, error) {
	if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		return true, nil
	}
	br := bufio.NewReader(r.Body)
	if _, err := br.Peek(1); err != nil {
		if err == io.EOF {
			return true, nil
		}
		return false, err
	}
	r.Body = peekedBody{Reader: br, Closer: r.Body}
	return false, nil
}

// Reset melayani request reset password.
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	var req request
	empty, err := bodyIsEmpty(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !empty {
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	res, err := h.uc.Execute(r.Context(), Input{
		TargetID: chi.URLParam(r, "id"),
		Password: req.Password,
		Meta:     base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())},
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, res)
}
