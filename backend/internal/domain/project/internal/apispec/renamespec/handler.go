package renamespec

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

type renameResponse struct {
	Spec base.SpecDTO `json:"spec"`
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(r.Context()))
	if !ok {
		respond.WriteError(w, r, errs.New(errs.CodeUnauthorized, "perlu login"))
		return
	}
	newName, err := parseRename(r)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	sum, err := h.uc.Execute(r.Context(), Input{
		ProjectID: chi.URLParam(r, "projectID"),
		OldName:   chi.URLParam(r, "name"),
		NewName:   newName,
		Actor:     actor,
		Meta:      base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent()),
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, renameResponse{Spec: base.ToSummaryDTO(sum)})
}
