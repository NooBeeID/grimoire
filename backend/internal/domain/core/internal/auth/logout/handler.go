package logout

import (
	"net/http"

	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

// Handler melayani POST /api/auth/logout.
type Handler struct {
	uc           *Usecase
	cookieSecure bool
}

// NewHandler membuat Handler.
func NewHandler(uc *Usecase, cookieSecure bool) *Handler {
	return &Handler{uc: uc, cookieSecure: cookieSecure}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.Logout(r.Context(), parseParams(r)); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: middleware.SessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.cookieSecure,
	})
	w.WriteHeader(http.StatusNoContent)
}
