package changepassword

import (
	"net/http"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
)

// Handler melayani POST /api/auth/change-password.
type Handler struct {
	uc     *Usecase
	secret []byte
}

// NewHandler membuat Handler.
func NewHandler(uc *Usecase, secret []byte) *Handler { return &Handler{uc: uc, secret: secret} }

// ChangePassword melayani request ganti password.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	c, err := r.Cookie(middleware.SessionCookieName)
	if err != nil || c.Value == "" {
		httpx.WriteError(w, r, errs.New(errs.CodeUnauthorized, "perlu login"))
		return
	}
	err = h.uc.Execute(r.Context(), Input{
		CurrentPassword:  req.CurrentPassword,
		NewPassword:      req.NewPassword,
		CurrentTokenHash: base.HashToken(h.secret, c.Value),
		Meta:             base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())},
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
