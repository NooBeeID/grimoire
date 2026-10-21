package uploadspecs

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
)

type Executor interface {
	Execute(ctx context.Context, in Input) (Output, error)
}

type Handler struct {
	uc           Executor
	maxFileBytes int64
	maxFiles     int
}

type uploadResponse struct {
	Results []base.FileResultDTO  `json:"results"`
	Summary base.UploadSummaryDTO `json:"summary"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	form, err := upload.Read(r, h.maxFileBytes, h.maxFiles)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	out, err := h.uc.Execute(r.Context(), Input{
		ProjectID: chi.URLParam(r, "projectID"),
		Files:     form.Files,
		Name:      form.Name,
		Meta:      base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent()),
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	results := make([]base.FileResultDTO, len(out.Results))
	for i, fr := range out.Results {
		results[i] = base.ToFileResultDTO(fr)
	}
	respond.WriteJSON(w, http.StatusOK, uploadResponse{Results: results, Summary: base.UploadSummaryDTO(out.Summary)})
}
