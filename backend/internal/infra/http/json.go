package httpx

import (
	"net/http"

	"grimoire/internal/infra/http/respond"
)

// WriteJSON menulis v sebagai JSON dengan status.
func WriteJSON(w http.ResponseWriter, status int, v any) { respond.WriteJSON(w, status, v) }

// DecodeJSON men-decode body JSON secara ketat (lihat respond.DecodeJSON).
func DecodeJSON(r *http.Request, dst any) error { return respond.DecodeJSON(r, dst) }
