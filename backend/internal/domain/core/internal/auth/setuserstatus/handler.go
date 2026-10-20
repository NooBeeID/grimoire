package setuserstatus

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
)

// Handler melayani POST /api/users/{id}/deactivate dan /activate (tanpa body).
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Deactivate menonaktifkan user.
func (h *Handler) Deactivate(w http.ResponseWriter, r *http.Request) { h.serve(w, r, false) }

// Activate mengaktifkan user.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) { h.serve(w, r, true) }

func (h *Handler) serve(w http.ResponseWriter, r *http.Request, active bool) {
	dto, err := h.uc.Execute(r.Context(), Input{
		TargetID: chi.URLParam(r, "id"),
		Active:   active,
		Meta:     base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())},
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, dto)
}
