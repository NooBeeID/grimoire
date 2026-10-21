package resendinvitation

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
)

// Handler melayani POST /api/users/{id}/resend-invitation.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Resend menerbitkan ulang undangan. Body diabaikan.
func (h *Handler) Resend(w http.ResponseWriter, r *http.Request) {
	in := Input{
		UserID: chi.URLParam(r, "id"), Via: ViaUsers,
		Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())},
	}
	if id, ok := reqctx.IdentityFrom(r.Context()); ok {
		in.InvitedBy = id.UserID
		in.InvitedByLabel = id.Name
		if in.InvitedByLabel == "" {
			in.InvitedByLabel = id.Email
		}
	}
	res, err := h.uc.Execute(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if res.Credentials != nil {
		w.Header().Set("Cache-Control", "no-store")
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}
