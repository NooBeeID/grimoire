package getconfig

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// params adalah input terurai dari request (belum divalidasi domain).
type params struct {
	ProjectID string
	RawKind   string
}

func parseParams(r *http.Request) params {
	return params{ProjectID: chi.URLParam(r, "projectID"), RawKind: strings.TrimSpace(r.URL.Query().Get("kind"))}
}
