package validatespec

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
	"grimoire/internal/infra/http/respond"
)

type Handler struct {
	uc           *Usecase
	maxFileBytes int64
	maxFiles     int
}

func newHandler(uc *Usecase, maxFileBytes int64, maxFiles int) *Handler {
	return &Handler{uc: uc, maxFileBytes: maxFileBytes, maxFiles: maxFiles}
}

type response struct {
	Results []base.FileResultDTO    `json:"results"`
	Summary base.ValidateSummaryDTO `json:"summary"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	form, err := upload.Read(r, h.maxFileBytes, h.maxFiles)
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	name := form.Name
	if name != nil {
		t := strings.TrimSpace(*name)
		if t == "" {
			name = nil
		} else {
			name = &t
		}
	}
	res, err := h.uc.Execute(r.Context(), Params{
		ProjectID: chi.URLParam(r, "projectID"), Name: name, Files: form.Files,
	})
	if err != nil {
		respond.WriteError(w, r, err)
		return
	}
	out := response{Results: make([]base.FileResultDTO, len(res.Results)), Summary: res.Summary}
	for i, fr := range res.Results {
		out.Results[i] = base.ToFileResultDTO(fr)
	}
	respond.WriteJSON(w, http.StatusOK, out)
}
