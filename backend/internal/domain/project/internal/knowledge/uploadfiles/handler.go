package uploadfiles

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type executor interface {
	Authorize(ctx context.Context, projectID string) (base.Actor, error)
	Execute(ctx context.Context, actor base.Actor, in Input) (base.UploadResultDTO, error)
}

type Handler struct {
	uc     executor
	limits base.Limits
	// read dapat diganti di test; default readParts.
	read func(*http.Request, base.Limits) (Parsed, error)
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	pid := chi.URLParam(r, "projectID")
	actor, err := h.uc.Authorize(r.Context(), pid)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	read := h.read
	if read == nil {
		read = readParts
	}
	parsed, err := read(r, h.limits)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	res, err := h.uc.Execute(r.Context(), actor, Input{ProjectID: pid, Parts: parsed.Parts, Meta: meta})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, res)
}
