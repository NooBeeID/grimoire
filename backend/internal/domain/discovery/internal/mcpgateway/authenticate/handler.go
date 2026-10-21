package authenticate

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/lib/common/reqctx"
)

type handler struct {
	uc               *Usecase
	body401, body503 []byte
	log              *slog.Logger
}

func newHandler(uc *Usecase, log *slog.Logger) *handler {
	if log == nil {
		log = slog.Default()
	}
	b401, _ := json.Marshal(base.HTTPReject(base.CodeUnauthorized, "Unauthorized", nil))
	b503, _ := json.Marshal(base.HTTPReject(base.CodeUnavailable, "Service unavailable", nil))
	return &handler{uc: uc, body401: b401, body503: b503, log: log}
}

func (h *handler) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := h.uc.Execute(r.Context(), ParseInput(r))
		switch d.Kind {
		case Admitted:
			ctx := reqctx.WithAPIKeyID(r.Context(), d.Principal.KeyID)
			ctx = base.WithPrincipal(ctx, d.Principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		case Unauthorized:
			w.Header().Set("WWW-Authenticate", `Bearer realm="grimoire-mcp"`)
			h.reject(w, r, http.StatusUnauthorized, h.body401)
		case RateLimited:
			w.Header().Set("Retry-After", strconv.Itoa(d.RetryAfter))
			body, _ := json.Marshal(base.HTTPReject(base.CodeRateLimited, "Rate limit exceeded", map[string]int{"retry_after_seconds": d.RetryAfter}))
			h.reject(w, r, http.StatusTooManyRequests, body)
		case Unavailable:
			h.reject(w, r, http.StatusServiceUnavailable, h.body503)
		default:
			h.log.ErrorContext(r.Context(), "mcp authenticate: decision tidak dikenal", "kind", int(d.Kind))
			h.reject(w, r, http.StatusServiceUnavailable, h.body503)
		}
	})
}

func (h *handler) reject(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		h.log.DebugContext(r.Context(), "mcp authenticate: write gagal", "err", err)
	}
}
