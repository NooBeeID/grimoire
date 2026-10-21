package inviteuser

import (
	"net/http"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Handler melayani POST /api/users/invite.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Invite mengundang developer lewat email.
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	var req InviteRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	actor, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, errs.New(errs.CodeUnauthorized, "perlu login"))
		return
	}
	res, err := h.uc.Execute(r.Context(), Input{
		Email: req.Email, Name: req.Name,
		InvitedBy: actor.UserID, InvitedByLabel: inviterLabel(actor),
		Deliver: true, Via: ViaUsers,
		Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())},
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if res.Credentials != nil {
		w.Header().Set("Cache-Control", "no-store")
	}
	httpx.WriteJSON(w, http.StatusCreated, toResponse(res, res.Now))
}

func inviterLabel(a reqctx.Identity) string {
	if a.Name != "" {
		return a.Name
	}
	return a.Email
}
