package listusers

import (
	"net/http"

	"grimoire/internal/domain/core/internal/auth/base"
	httpx "grimoire/internal/infra/http"
)

// Handler melayani GET /api/users.
type Handler struct{ uc *Usecase }

// NewHandler membuat Handler.
func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// List mem-parse query, memanggil usecase, dan menulis JSON (tanpa cache).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	in, fe := parseQuery(r.URL.Query())
	if len(fe) > 0 {
		httpx.WriteError(w, r, base.InvalidFields(fe))
		return
	}
	out, err := h.uc.Execute(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, out)
}
