package downloadspec

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/http/respond"
)

// Handler menulis byte spec langsung (bukan WriteJSON).
type Handler struct {
	uc  *Usecase
	log *slog.Logger
}

func newHandler(uc *Usecase, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{uc: uc, log: log}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := Params{ProjectID: chi.URLParam(r, "projectID"), Name: chi.URLParam(r, "name")}
	f, err := h.uc.Execute(r.Context(), p)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", f.MediaType)
	hd.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.FileName))
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "no-store")
	hd.Set("Content-Length", strconv.Itoa(len(f.Content)))
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, f.Content); err != nil {
		h.log.WarnContext(r.Context(), "downloadspec: tulis respons gagal", "feature", "downloadspec", "error", err)
	}
}
