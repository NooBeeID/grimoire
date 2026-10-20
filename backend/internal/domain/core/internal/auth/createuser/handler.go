package createuser

import (
	"net/http"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
)

// Handler melayani POST /api/users.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Create membuat user baru.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := h.uc.Execute(r.Context(), Input{
		Email: req.Email, Name: req.Name, Role: req.Role, Password: req.Password,
		Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())},
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusCreated, res)
}
