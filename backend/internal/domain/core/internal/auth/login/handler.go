package login

import (
	"errors"
	"net/http"
	"strconv"
	"unicode/utf8"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
)

// Handler adalah adapter HTTP.
type Handler struct {
	uc           *Usecase
	cookieSecure bool
}

// NewHandler membuat Handler.
func NewHandler(uc *Usecase, cookieSecure bool) *Handler {
	return &Handler{uc: uc, cookieSecure: cookieSecure}
}

// Login melayani POST /api/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), base.MaxUserAgentBytes)}
	res, err := h.uc.Execute(r.Context(), Input{Email: req.Email, Password: req.Password, Meta: meta})
	if err != nil {
		if secs, ok := retryAfterSeconds(err); ok {
			w.Header().Set("Retry-After", strconv.Itoa(secs))
		}
		httpx.WriteError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: middleware.SessionCookieName, Value: res.Token, Path: "/", MaxAge: res.MaxAge,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.cookieSecure,
	})
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, LoginResponse{User: res.User})
}

// retryAfterSeconds mengambil retry_after_seconds dari error ACCOUNT_LOCKED.
func retryAfterSeconds(err error) (int, bool) {
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != base.CodeAccountLocked {
		return 0, false
	}
	d, ok := e.Details.(map[string]any)
	if !ok {
		return 0, false
	}
	n, ok := d["retry_after_seconds"].(int)
	if !ok || n < 1 {
		return 0, false
	}
	return n, true
}

// truncate memotong s hingga maks max byte pada batas rune.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
