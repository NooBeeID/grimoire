package runsync

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
)

type handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in RunRequest
	if err := respond.DecodeJSON(r, &in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), in, meta)
	if err != nil {
		setRetryAfter(w, err)
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, res)
}

// setRetryAfter menyalin details.retry_after_seconds ke header Retry-After.
func setRetryAfter(w http.ResponseWriter, err error) {
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeRateLimited {
		return
	}
	d, ok := e.Details.(map[string]any)
	if !ok {
		return
	}
	if n, ok := d["retry_after_seconds"].(int); ok && n > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(n))
	}
}
