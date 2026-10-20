package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Pinger memeriksa keterjangkauan dependensi (mis. DB).
type Pinger interface {
	Ping(ctx context.Context) error
}

type healthBody struct {
	Status string `json:"status"`
}

func healthz(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, healthBody{Status: "ok"})
}

func readyz(p Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p == nil {
			WriteJSON(w, http.StatusServiceUnavailable, healthBody{Status: "unavailable"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := p.Ping(ctx); err != nil {
			slog.WarnContext(r.Context(), "readyz: ping gagal", "error", err)
			WriteJSON(w, http.StatusServiceUnavailable, healthBody{Status: "unavailable"})
			return
		}
		WriteJSON(w, http.StatusOK, healthBody{Status: "ok"})
	}
}
